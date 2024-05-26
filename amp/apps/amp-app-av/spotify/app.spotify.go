package spotify

import (
	"net/url"
	"sync"

	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/av"
	"github.com/amp-3d/amp-host-go/amp/apps/amp-app-av/spotify/oauth"
	respot "github.com/amp-3d/amp-librespot-go/librespot/api-respot"
	_ "github.com/amp-3d/amp-librespot-go/librespot/core" // bootstrap
	"github.com/amp-3d/amp-sdk-go/amp"
	"github.com/amp-3d/amp-sdk-go/amp/basic"
	"github.com/amp-3d/amp-sdk-go/amp/registry"
	"github.com/amp-3d/amp-sdk-go/stdlib/tag"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

func init() {
	reg := registry.Global()

	reg.RegisterApp(&amp.App{
		AppSpec: tag.FormSpec(av.AppSpec, "spotify"),
		Desc:    "client for Spotify",
		Version: "v1.2023.2",
		NewAppInstance: func(ctx amp.AppContext) (amp.AppInstance, error) {
			app := &appInst{
				sessReady: make(chan struct{}),
			}
			app.Instance = app
			app.AppContext = ctx
			return app, nil
		},
	})
}

const (
	//kRedirectURL = "http://localhost:5000/callback"
	kRedirectURL         = "nodle-music-player://spotify/auth" // TODO: sent in from client -- check me?
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

type appInst struct {
	basic.App[*appInst]
	sessReady chan struct{}        // closed when session is established
	client    *spotify.Client      // nil if not signed in
	me        *spotify.PrivateUser // nil if not signed in
	respot    respot.Session       // nil if not signed in
	auth      *oauth.Config
	sessMu    sync.Mutex
}

func (app *appInst) OnClosing() {
	app.endSession()
}

func (app *appInst) handleAuthURL(url *url.URL) error {

	if app.auth == nil {
		return amp.ErrCode_NotConnected.Error("unexpected auth")
	}

	// Redeem the auth code and store the latest token.
	// Blocks but ok since this is called in app's Context
	token, err := app.auth.Exchange(app, "", url)
	if err == nil {
		err = app.auth.OnTokenUpdated(token, true)
		if err == nil {
			err = app.tryConnect()
		}
	}

	return err
}

// Optionally blocks until the spotify session is ready (or AppContext is closing)
func (app *appInst) waitForSession() error {
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
		return amp.ErrShuttingDown
	}
}

func (app *appInst) tryConnect() error {
	app.sessMu.Lock() // needed?
	defer app.sessMu.Unlock()

	app.resetSignal()

	if app.auth == nil {
		app.auth = oauth.NewAuth(app, oauthConfig)
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
		err = amp.ErrCode_ProviderErr.Errorf("failed to get current user: %v", err)
	}
	if err != nil {
		return err
	}

	// Unclear on why we need to guard on this
	if app.auth == nil {
		return amp.ErrShuttingDown
	}

	// At this point we have a token -- TODO it may be expired
	if token := app.auth.CurrentToken(); token != nil {
		if app.respot == nil {
			info := app.Session().Auth()
			ctx := respot.DefaultSessionContext(info.DeviceLabel)
			ctx.Context = app
			app.respot, err = respot.StartNewSession(ctx)
			if err != nil {
				err = amp.ErrCode_ProviderErr.Errorf("StartSession error: %v", err)
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
func (app *appInst) resetSignal() {
	select {
	case <-app.sessReady:
	default:
		if app.sessReady == nil {
			app.sessReady = make(chan struct{})
		}
	}
}

func (app *appInst) endSession() {
	app.resetSignal()

	if app.client != nil {
		app.me = nil
		app.client = nil
		app.auth = nil
	}
	if app.respot != nil {
		app.respot.Close()
		app.respot = nil
	}
}

func (app *appInst) MakeReady(op amp.Requester) error {
	url := op.Request().URL
	if url != nil && url.Path != "/auth" {
		err := app.handleAuthURL(url)
		return err
	}
	return app.waitForSession()
}

func (app *appInst) ServeRequest(op amp.Requester) (amp.Pin, error) {
	cell := &spotifyHome{}
	cell.Tab = amp.TagTab{
		Label: "Spotify",
		Tags: []*amp.Tag{
			amp.PinnableCatalog,
		},
	}
	return app.PinAndServe(cell, op)
}
