package amp_spotify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arcspace/go-arc-sdk/stdlib/platform"
	"github.com/arcspace/go-archost/arc"
	"github.com/arcspace/go-archost/arc/apps/amp/api"
	respot "github.com/arcspace/go-librespot/librespot/api-respot"
	_ "github.com/arcspace/go-librespot/librespot/core" // bootstrap
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

const (
	AppID = "arcspace.systems.app.amp.spotify"
)

func init() {
	arc.RegisterApp(&arc.AppModule{
		AppID:   AppID,
		Version: "v1.2023.2",
		NewAppInstance: func(ctx arc.AppContext) (arc.AppRuntime, error) {
			app := &appCtx{
				AppContext: ctx,
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

type appCtx struct {
	arc.AppContext
	client        *spotify.Client // nil if not signed in
	token         oauth2.Token
	tokenLoaded   bool // set if token is loaded (regardless of validity)
	loginAttempts int
	me            *spotify.PrivateUser
	auth          *spotifyauth.Authenticator
	home          AmpCell
	respot        respot.Session
	sessReady     chan struct{} // closed when login is complete
}

func (app *appCtx) AppID() string {
	return api.AmpAppURI
}

func (app *appCtx) OnClosing() {
}

const kTokenNameID = ".oauth.Token"

func (app *appCtx) HandleAppMsg(m *arc.AppMsg) (handled bool, err error) {
	if m.MsgType == arc.AppMsgType_URI {
		uri := m.Entries["uri"]
		if strings.Contains(uri, "://spotify/auth") {
			var err error
			faux := http.Request{}
			faux.URL, err = url.Parse(uri)
			if err != nil {
				return true, err
			}

			token, err := app.auth.Token(app, "", &faux)
			if err == nil {
				var tokenJson []byte
				tokenJson, err = json.Marshal(token)
				if err == nil {
					err = app.PutAppValue(kTokenNameID, tokenJson)
				}
			}
			if err != nil {
				return true, err
			}

			// TODO: handle race condition
			app.tokenLoaded = false
			app.retryLogin()

			// if err != nil {
			// 	return // TODO: alert of failure
			// }

			/*
				app.StartChild(&process.Task{
					Label:     "retrieving token",
					IdleClose: time.Nanosecond,
					OnRun: func(ctx process.Context) {
						token, err := app.auth.Token(app, "", &faux)
						if err != nil {
							return // TODO: alert of failure
						}
						//tryLogin(token oauth2.Token, isNewToken bool) error
						var cookie sessCookie
						cookie.TokenJSON, err = json.Marshal(token)
						if err != nil {
							return // TODO: alert of failure
						}
						err = arc.WriteCell(app, kCookieNameID, &cookie)
						if err != nil {
							return // TODO: alert of failure
						}
						// TODO: alert of new token
					},
				})
			*/
		}
	}
	return false, nil
}

// Blocks until the spotify session is ready (or app is told to closE)
func (app *appCtx) waitForSession() error {

	select {
	case <-app.sessReady:
		return nil
	case <-app.Closing():
		return arc.ErrShuttingDown
	default:
	}

	for {
		err := app.retryLogin()
		if err != nil {
			return err
		}

		select {
		case <-app.sessReady:
			return nil
		case <-app.Closing():
			return arc.ErrShuttingDown
		}
	}
}

func (app *appCtx) retryLogin() error {
	var err error

	app.close()

	if app.auth == nil {

		/*
			auth := &spotifyauth.Authenticator{
				config: &oauth2.Config{
					ClientID:     kSpotifyClientID,
					ClientSecret: kSpotifyClientSecret,
					RedirectURL: kRedirectURL,
					Endpoint: oauth2.Endpoint{
						AuthURL:  spotifyauth.AuthURL,
						TokenURL: spotifyauth.TokenURL,
					},
					Scopes: []string{
						spotifyauth.ScopeUserReadPrivate,
						spotifyauth.ScopeUserReadCurrentlyPlaying,
						spotifyauth.ScopeUserReadPlaybackState,
						spotifyauth.ScopeUserModifyPlaybackState,
						spotifyauth.ScopeStreaming,
					},
				},
			}

			for _, opt := range opts {
				opt(a)
			}
		*/

		app.auth = spotifyauth.New(
			spotifyauth.WithClientID(kSpotifyClientID),
			spotifyauth.WithClientSecret(kSpotifyClientSecret),
			spotifyauth.WithRedirectURL(kRedirectURL),
			spotifyauth.WithScopes(
				spotifyauth.ScopeUserReadPrivate,
				spotifyauth.ScopeUserReadCurrentlyPlaying,
				spotifyauth.ScopeUserReadPlaybackState,
				spotifyauth.ScopeUserModifyPlaybackState,
				spotifyauth.ScopeStreaming,
			),
		)
	}

	// If we need a token, launch the auth URL and wait
	if !app.tokenLoaded {
		err = app.loadStoredToken()
		if err != nil {
			url := app.auth.AuthURL("")
			fmt.Println("Launching ", url)
			platform.LaunchURL(url)
			return nil
		}
	}

	{
		if app.respot == nil {
			info := app.User().LoginInfo()
			ctx := respot.DefaultSessionCtx(info.DeviceLabel)
			ctx.Context = app
			app.respot, err = respot.StartNewSession(ctx)
			if err != nil {
				err = arc.ErrCode_ProviderErr.Errorf("StartSession error: %v", err)
				app.Warn(err)
			}

			if err == nil {
				ctx.Login.AuthToken = app.token.AccessToken
				err = app.respot.Login()
			}
		}
	}

	if err != nil {
		return err
	}

	// use the token to get an authenticated client
	app.client = spotify.New(app.auth.Client(app, &app.token))
	app.me, err = app.client.CurrentUser(app)
	if err != nil {
		app.client = nil
		err = arc.ErrCode_ProviderErr.Error("failed to get current user")
	}

	if err != nil {
		return err
	}

	// signal that that the session is ready!
	close(app.sessReady)

	return nil
}

func (app *appCtx) close() {
	app.home = nil
	app.me = nil

	// TODO: Hacky but works for now
	oldSignal := app.sessReady
	app.sessReady = make(chan struct{})
	if oldSignal != nil {
		select {
		case <-oldSignal:
		default:
			close(oldSignal)
		}
	}

	app.client = nil

	//app.client.Close

	// 	for waiting := true; waiting; {
	// 		select {
	// 		case oldSignal<-struct{}{}:
	// 		default:
	// 			waiting = false
	// 		}
	// 	}
	// }

}

func (app *appCtx) loadStoredToken() error {

	app.tokenLoaded = true
	app.token = oauth2.Token{}
	tokenJson, err := app.GetAppValue(kTokenNameID)
	if err == nil {
		err = json.Unmarshal(tokenJson, &app.token)
		if err == nil {
			if app.token.Expiry.Before(time.Now()) {
				err = arc.ErrCode_SessionExpired.Error("Spotify token expired")
			}
		}
	}
	return err
}

func (app *appCtx) signOut(user arc.User) {
	if app.client != nil {
		app.client = nil
	}
	app.me = nil
	//app.sessReady = nil
}

func (app *appCtx) PinCell(req *arc.CellReq) (arc.AppCell, error) {

	// TODO: regen home once token arrives
	if app.home == nil {
		app.home = newRootCell(app)
		return app.home, nil
	}

	return nil, nil
}
