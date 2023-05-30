package amp_spotify

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
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
	sessReady   chan struct{}        // closed once session is established
	client      *spotify.Client      // nil if not signed in
	me          *spotify.PrivateUser // nil if not signed in
	respot      respot.Session       // nil if not signed in
	token       oauth2.Token
	auth        *spotifyauth.Authenticator
	home        AmpCell
	isConnected bool
}

func (app *appCtx) OnClosing() {
	app.endSession()
}

const kTokenNameID = ".oauth.Token.v5"

func (app *appCtx) HandleMetaMsg(msg *arc.Msg) (handled bool, err error) {
	if msg.ValType == arc.ValType_HandleURI {
		uri := string(msg.ValBuf)
		if strings.Contains(uri, "://spotify/auth") {
			var err error
			faux := http.Request{}
			faux.URL, err = url.Parse(uri)
			if err != nil {
				return true, err
			}

			// redeem the code for the token and store it as the latest token -- this is blocking but handled properly since we pass in the app's process.Context
			token, err := app.auth.Token(app, "", &faux)
			if err == nil {
				var tokenJson []byte
				tokenJson, err = json.Marshal(token)
				if err == nil {
					if err = app.PutAppValue(kTokenNameID, tokenJson); err == nil {
						app.Info(2, "wrote ", kTokenNameID)
						err = app.trySession()
					}
				}
			}

			return true, err
		}
	}
	return false, nil
}

// Blocks until the spotify session is ready (or app is told to closE)
func (app *appCtx) waitForSession() error {

	select {
	case <-app.sessReady:
		return nil
	default:
		err := app.trySession()
		if err != nil {
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

func (app *appCtx) trySession() error {
	if app.isConnected {
		return nil
	}
	
	if app.sessReady == nil {
		app.sessReady = make(chan struct{})
	}

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

	var err error

	// If there's an error loading the token, push a oauth URL to the client
	{
		err = app.loadStoredToken()
		if err != nil {
			url := app.auth.AuthURL("")
			msg := arc.NewMsg()
			msg.SetValBuf(arc.ValType_HandleURI, []byte(url))
			app.User().PushMetaMsg(msg)
			return nil
		}
	}

	// At this point we have a token -- TODO it may be expired
	{
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

func (app *appCtx) endSession() {
	app.sessReady = nil
	
	if app.isConnected {
		app.home = nil
		app.me = nil
		app.client = nil
		app.auth = nil
		if app.respot != nil {
			app.respot.Close()
			app.respot = nil
		}
		app.isConnected = false
	}
	// // TODO: Hacky but works for now
	// oldSignal := app.sessReady
	// app.sessReady = make(chan struct{})
	// if oldSignal != nil {
	// 	select {
	// 	case <-oldSignal:
	// 	default:
	// 		close(oldSignal)
	// 	}
	// }

	app.client = nil
}

func (app *appCtx) loadStoredToken() error {
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

func (app *appCtx) PinCell(req *arc.CellReq) (arc.AppCell, error) {

	// TODO: regen home once token arrives
	if app.home == nil {
		app.home = newRootCell(app)
		return app.home, nil
	}

	return nil, nil
}
