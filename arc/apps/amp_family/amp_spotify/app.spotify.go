package amp_spotify

import (
	"net/url"
	"strings"
	"sync"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp_spotify/oauth"
	respot "github.com/arcspace/go-librespot/librespot/api-respot"
	_ "github.com/arcspace/go-librespot/librespot/core" // bootstrap
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

const (
	AppURI = amp.AppFamily + "spotify/v1"
)

func UID() arc.UID {
	return arc.FormUID(0x75f4138f0e984d89, 0x968a9b7d7ffa10cd)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
		URI:     AppURI,
		UID:     UID(),
		Desc:    "client for Spotify",
		Version: "v1.2023.2",
		NewAppInstance: func(ctx arc.AppContext) (arc.AppRuntime, error) {
			app := &appCtx{
				AppContext: ctx,
				sessReady:  make(chan struct{}),
			}
			return app, nil
		},
	})
}

const (
	//kRedirectURL = "http://localhost:5000/callback"
	kRedirectURL         = "nodle-music-player://spotify/auth" // TODO: sent in from client
	kSpotifyClientID     = "8de730d205474e1490e696adfc10d61c"
	kSpotifyClientSecret = "d8cd8d502b6f4ecda140b6fffa1a9f9f"
)

var oauthConfig = oauth2.Config{
	ClientID:     kSpotifyClientID,
	ClientSecret: kSpotifyClientSecret,
	RedirectURL:  kRedirectURL,
	Endpoint: oauth2.Endpoint{
		AuthURL:  spotifyauth.AuthURL,
		TokenURL: spotifyauth.TokenURL,
	},
	Scopes: []string{
		spotifyauth.ScopeStreaming,
		spotifyauth.ScopeUserTopRead,
		spotifyauth.ScopeUserReadPrivate,
		spotifyauth.ScopeUserLibraryRead,
		spotifyauth.ScopeUserLibraryModify,
		spotifyauth.ScopeUserFollowRead,
		spotifyauth.ScopeUserFollowModify,
		spotifyauth.ScopeUserReadRecentlyPlayed,
		spotifyauth.ScopePlaylistReadPrivate,
		spotifyauth.ScopePlaylistModifyPublic,
		spotifyauth.ScopePlaylistReadCollaborative,
	},
}

type appCtx struct {
	arc.AppContext
	sessReady chan struct{}        // closed when session is established
	client    *spotify.Client      // nil if not signed in
	me        *spotify.PrivateUser // nil if not signed in
	respot    respot.Session       // nil if not signed in
	auth      *oauth.Config
	home      AmpCell
	sessMu    sync.Mutex
}

func (app *appCtx) OnClosing() {
	app.endSession()
}

func (app *appCtx) HandleMetaMsg(msg *arc.Msg) (handled bool, err error) {
	if msg.ValType == arc.ValType_HandleURI {
		uri := string(msg.ValBuf)
		if strings.Contains(uri, "://spotify/auth") {
			url, err := url.Parse(uri)
			if err != nil {
				return true, err
			}

			// redeem the code for the token and store it as the latest token -- this is blocking but handled properly since we pass in the app's process.Context
			token, err := app.auth.Exchange(app, "", url)
			if err == nil {
				err = app.auth.OnTokenUpdated(token, true)
				if err == nil {
					err = app.tryConnect()
				}
			}

			return true, err
		}
	}
	return false, nil
}

// Optionally blocks until the spotify session is ready (or AppContext is closing)
func (app *appCtx) waitForSession() error {
	select {
	case <-app.sessReady:
		return nil
	default:
		if err := app.tryConnect(); err != nil {
			return err
		}
	}

	select {
	case <-app.sessReady:
		return nil
	case <-app.Closing():
		return arc.ErrShuttingDown
	}
}

func (app *appCtx) tryConnect() error {
	app.sessMu.Lock() // needed?
	defer app.sessMu.Unlock()

	app.resetSignal()

	if app.auth == nil {
		app.auth = oauth.NewAuth(app.AppContext, oauthConfig)
	}

	// If we don't have a token, ask the client launch a URL that will start oauth
	// If we have a token, but it's expired it will auto renew within the http.Client that is created
	if app.auth.AwaitingAuth(true) {
		return nil
	}

	if app.client == nil {
		app.client = spotify.New(app.auth.NewHttpClient())
	}

	// TODO: if we get an error, perform a full reauth
	var err error
	app.me, err = app.client.CurrentUser(app)
	if err != nil {
		app.client = nil
		err = arc.ErrCode_ProviderErr.Error("failed to get current user")
	}
	if err != nil {
		return err
	}

	// At this point we have a token -- TODO it may be expired
	if token := app.auth.CurrentToken(); token != nil {
		if app.respot == nil {
			info := app.User().LoginInfo()
			ctx := respot.DefaultSessionContext(info.DeviceLabel)
			ctx.Context = app
			app.respot, err = respot.StartNewSession(ctx)
			if err != nil {
				err = arc.ErrCode_ProviderErr.Errorf("StartSession error: %v", err)
				app.Warn(err)
			}

			if err == nil {
				ctx.Login.AuthToken = token.AccessToken
				err = app.respot.Login()
			}
		}
	}

	if err != nil {
		return err
	}

	// signal that that the session is ready!
	select {
	case <-app.sessReady:
	default:
		close(app.sessReady)
	}

	return nil
}

// Resets the "session is ready" signal if needed
func (app *appCtx) resetSignal() {
	select {
	case <-app.sessReady:
	default:
		if app.sessReady == nil {
			app.sessReady = make(chan struct{})
		}
	}
}

func (app *appCtx) endSession() {
	app.resetSignal()

	if app.client != nil {
		app.home = nil
		app.me = nil
		app.client = nil
		app.auth = nil
	}
	if app.respot != nil {
		app.respot.Close()
		app.respot = nil
	}
}

func (app *appCtx) PinCell(req *arc.CellReq) (arc.Cell, error) {

	// TODO: regen home once token arrives?
	if app.home == nil {
		cell := newRootCell(app)
		app.home = cell
		return app.home, nil
	}

	return nil, nil
}
