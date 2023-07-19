package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/arcspace/go-arc-sdk/apis/arc"
	"golang.org/x/oauth2"
)

const kTokenAttrSpec = "AuthToken:primary"

type Config struct {
	Config oauth2.Config
	renew  oauth2.TokenSource // renews a token automatically when expired
	ctx    arc.AppContext
	token  *oauth2.Token
}

// ShowDialog forces the user to approve the app, even if they have already done so.
// Without this, users who have already approved the app are immediately redirected to the redirect uri.
var ShowDialog = oauth2.SetAuthURLParam("show_dialog", "true")

func NewAuth(ctx arc.AppContext, config oauth2.Config) *Config {
	auth := &Config{
		ctx:    ctx,
		Config: config,
	}
	return auth
}

// Returns true if no auth token is present.
// If autoRequest is true and no token is present, the client is msged to to launch the entry auth URL that starts oauth flow.
func (auth *Config) AwaitingAuth(autoRequest bool) bool {

	// If we already have a token or get one from storage, we're good
	if auth.token != nil {
		return false
	}
	err := auth.readStoredToken()
	if err == nil {
		return false
	}

	// Atr this point, we know we need to initiate oauth flow
	if autoRequest {
		auth.pushAuthCodeRequest()
	}
	return true
}

func (auth *Config) CurrentToken() *oauth2.Token {
	return auth.token
}

// Pushes a msg to the client to launch a URL that starts oauth flow.
func (auth *Config) pushAuthCodeRequest() error {
	elem := arc.AttrElem{
		Val: &arc.HandleURI{
			URI: auth.Config.AuthCodeURL(""),
		},
	}
	return arc.SendMetaAttr(auth.ctx.Session(), elem)
}



// NewHttpClient creates a *http.Client that will use the specified access token for its API requests.
// Combine this with spotify.HTTPClientOpt.
func (auth *Config) NewHttpClient() *http.Client {
	auth.renew = auth.Config.TokenSource(auth.ctx, auth.token)
	tokenSrc := oauth2.ReuseTokenSource(nil, auth)
	return oauth2.NewClient(auth.ctx, tokenSrc)
}

// Exchange converts an authorization code into a token.
func (auth *Config) Exchange(ctx context.Context, state string, uri *url.URL, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
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
	return auth.Config.Exchange(ctx, code, opts...)
}

func (auth *Config) readStoredToken() error {
	attr := arc.AuthToken{}

	err := auth.ctx.GetAppCellAttr(kTokenAttrSpec, &attr)
	if err != nil || (attr.AccessToken == "" && attr.RefreshToken == "") {
		return arc.ErrNoAuthToken
	}

	// fmt.Println("AccessToken:  ", tok.AccessToken)
	// fmt.Println("TokenType:    ", tok.TokenType)
	// fmt.Println("RefreshToken: ", tok.RefreshToken)
	// fmt.Println("Expiry:       ", tok.Expiry)
	auth.token = &oauth2.Token{
		AccessToken:  attr.AccessToken,
		TokenType:    attr.TokenType,
		RefreshToken: attr.RefreshToken,
		Expiry:       time.Unix(attr.Expiry, 0),
	}
	return nil
}

func (auth *Config) OnTokenUpdated(tok *oauth2.Token, saveToken bool) error {
	auth.token = tok

	if tok == nil {
		return arc.ErrCode_InternalErr.Error("oauth token is nil")
	}

	var err error
	if saveToken {
		attr := arc.AuthToken{
			AccessToken:  tok.AccessToken,
			TokenType:    tok.TokenType,
			RefreshToken: tok.RefreshToken,
			Expiry:       tok.Expiry.Unix(),
		}
		err = auth.ctx.PutAppCellAttr(kTokenAttrSpec, &attr)
		if err == nil {
			auth.ctx.Info(2, "wrote new oauth token")
		} else {
			auth.ctx.Error("error storing token:", err)
		}
	}
	return err
}

func (auth *Config) Token() (*oauth2.Token, error) {
	tok, err := auth.renew.Token()
	if err != nil {
		return nil, err
	}

	// Don't bother storing a token that has the same refresh token and expires soon
	saveToken := true
	if tok.RefreshToken == auth.token.RefreshToken {
		if !tok.Expiry.IsZero() && tok.Expiry.Add(-3*time.Hour).Before(time.Now()) {
			saveToken = false
		}
	}

	auth.OnTokenUpdated(tok, saveToken)
	return tok, nil
}
