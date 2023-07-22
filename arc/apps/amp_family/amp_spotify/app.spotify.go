package amp_spotify

import (
	"net/url"
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
	AppID = "spotify" + amp.AppFamilyDomain
)

func UID() arc.UID {
	return arc.FormUID(0x75f4138f0e984d89, 0x968a9b7d7ffa10cd)
}

func RegisterApp(reg arc.Registry) {
	reg.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		UID:     UID(),
		Desc:    "client for Spotify",
		Version: "v1.2023.2",
		NewAppInstance: func() arc.AppInstance {
			return &appCtx{
				sessReady: make(chan struct{}),
			}
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
	amp.AppBase
	sessReady chan struct{}        // closed when session is established
	client    *spotify.Client      // nil if not signed in
	me        *spotify.PrivateUser // nil if not signed in
	respot    respot.Session       // nil if not signed in
	auth      *oauth.Config
	sessMu    sync.Mutex
	// home      AmpCell
	// rootCells []*amp.CellBase[*appCtx]
}

func (app *appCtx) OnClosing() {
	app.endSession()
}

func (app *appCtx) HandleURL(url *url.URL) error {

	if url.Path != "/auth" {
		return arc.ErrCode_InvalidURI.Errorf("unexpected path: %q", url.Path)
	}

	// Redeem the auth code and store the latest token.
	// Blocks but ok since we pass in the app's Context
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
		err = arc.ErrCode_ProviderErr.Error("failed to get current user")
	}
	if err != nil {
		return err
	}

	// At this point we have a token -- TODO it may be expired
	if token := app.auth.CurrentToken(); token != nil {
		if app.respot == nil {
			info := app.Session().LoginInfo()
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
		app.me = nil
		app.client = nil
		app.auth = nil
	}
	if app.respot != nil {
		app.respot.Close()
		app.respot = nil
	}
}

func (app *appCtx) PinCell(parent arc.PinnedCell, req arc.PinReq) (arc.PinnedCell, error) {
	if err := app.waitForSession(); err != nil {
		return nil, err
	}

	if parent != nil {
		return parent.PinCell(req)
	} else {

		// For now, just always pin a new home (root) cell
		cell := app.newRootCell()
		return amp.NewPinnedCell[*appCtx](app, &cell.CellBase)
	}
}

func (app *appCtx) newRootCell() *spotifyCell {
	cell := &spotifyCell{}
	cell.CellID = app.IssueCellID()
	cell.CellSpec = app.LinkCellSpec
	cell.Self = cell

	cell.info = arc.CellLabels{
		Title: "Spotify Home",
		// Glyph: &arc.AssetRef{
		// },
	}
	cell.pinner = pin_appHome
	return cell
}
