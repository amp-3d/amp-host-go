package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"golang.org/x/oauth2"
)

const kTokenNameID = ".oauth2.Token"

type Config struct {
	Config       oauth2.Config
	renew        oauth2.TokenSource // renews a token automatically when expired
	ctx          arc.AppContext
	CurrentToken oauth2.Token
}

// ShowDialog forces the user to approve the app, even if they have already done so.
// Without this, users who have already approved the app are immediately redirected to the redirect uri.
var ShowDialog = oauth2.SetAuthURLParam("show_dialog", "true")

func NewAuth(ctx arc.AppContext, config oauth2.Config) *Config {
	cfg := &Config{
		ctx:    ctx,
		Config: config,
	}
	return cfg
}

// Client creates a *http.Client that will use the specified access token for its API requests.
// Combine this with spotify.HTTPClientOpt.
func (cfg *Config) NewClient(ctx arc.AppContext) *http.Client {
	cfg.renew = cfg.Config.TokenSource(ctx, &cfg.CurrentToken)
	tokenSrc := oauth2.ReuseTokenSource(nil, cfg)
	return oauth2.NewClient(ctx, tokenSrc)
}

// Exchange converts an authorization code into a token.
func (cfg *Config) Exchange(ctx context.Context, state string, uri *url.URL, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	values := uri.Query()
	if err := values.Get("error"); err != "" {
		return nil, errors.New("spotify: auth failed - " + err)
	}
	code := values.Get("code")
	if code == "" {
		return nil, errors.New("spotify: didn't get access code")
	}
	actualState := values.Get("state")
	if actualState != state {
		return nil, errors.New("spotify: redirect state parameter doesn't match")
	}
	return cfg.Config.Exchange(ctx, code, opts...)
}

func (cfg *Config) ReadStoredToken() error {
	tok := oauth2.Token{}

	tokenJson, err := cfg.ctx.GetAppValue(kTokenNameID)
	if err == nil {
		err = json.Unmarshal(tokenJson, &tok)
	}

	if err != nil || (tok.AccessToken == "" && tok.RefreshToken == "") {
		return arc.ErrNoAuthToken
	}

	// fmt.Println("AccessToken:  ", tok.AccessToken)
	// fmt.Println("TokenType:    ", tok.TokenType)
	// fmt.Println("RefreshToken: ", tok.RefreshToken)
	// fmt.Println("Expiry:       ", tok.Expiry)
	cfg.CurrentToken = tok
	return nil
}

func (cfg *Config) OnTokenUpdated(tok *oauth2.Token, saveToken bool) error {
	cfg.CurrentToken = *tok

	var err error
	if saveToken {
		tokenJson, err := json.Marshal(tok)
		if err == nil {
			err = cfg.ctx.PutAppValue(kTokenNameID, tokenJson)
			if err == nil {
				cfg.ctx.Info(2, "wrote new oauth token")
			} else {
				cfg.ctx.Error("error storing token:", err)
			}
		}
	}
	return err
}

func (cfg *Config) Token() (*oauth2.Token, error) {
	tok, err := cfg.renew.Token()
	if err != nil {
		return nil, err
	}

	// Don't bother storing a token that has the same refresh token and expires soon
	saveToken := true
	if tok.RefreshToken == cfg.CurrentToken.RefreshToken {
		if !tok.Expiry.IsZero() && tok.Expiry.Add(-3*time.Hour).Before(time.Now()) {
			saveToken = false
		}
	}

	cfg.OnTokenUpdated(tok, saveToken)
	return tok, nil
}
