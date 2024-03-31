package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/arcspace/go-arc-sdk/stdlib/platform"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

var (
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

var auth = spotifyauth.New(
	spotifyauth.WithClientID(kSpotifyClientID),
	spotifyauth.WithClientSecret(kSpotifyClientSecret),
	spotifyauth.WithRedirectURL(kRedirectURL),
	spotifyauth.WithScopes(
		spotifyauth.ScopeUserReadPrivate,
		spotifyauth.ScopeUserReadCurrentlyPlaying,
		spotifyauth.ScopeUserReadPlaybackState,
		spotifyauth.ScopeUserModifyPlaybackState,
		spotifyauth.ScopeStreaming,
	))

func TestLogin(t *testing.T) {

	// first start an HTTP server
	http.HandleFunc("/callback", completeAuth)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Got request for:", r.URL.String())
	})
	go func() {
		err := http.ListenAndServe(":5000", nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	url := auth.AuthURL(state)
	fmt.Println("Launching ", url)
	platform.LaunchURL(url)

	// wait for auth to complete
	client := <-ch

	// use the client to make calls that require authorization
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("You are logged in as:", user.ID)

	// a := spotifyauth.New(redirectURL, spotify.ScopeUserLibraryRead, spotify.ScopeUserFollowRead)
	// // direct user to Spotify to log in
	// http.Redirect(w, r, a.AuthURL("state-string"), http.StatusFound)

	// // then, in redirect handler:
	// token, err := a.Token(state, r)
	// client := a.Client(token)

	// exec.Command("open", "https://www.facebook.com/v11.0/dialog/oauth?client_id=123456&redirect_uri=https://example/com").Run()

	// time.Sleep(time.Second * 10)

}

func completeAuth(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusForbidden)
		log.Fatal(err)
	}
	if st := r.FormValue("state"); st != state {
		http.NotFound(w, r)
		log.Fatalf("State mismatch: %s != %s\n", st, state)
	}

	tokenJson, err := json.Marshal(tok)
	if err == nil {
		os.WriteFile("spotify_token.json", tokenJson, 0666)
		fmt.Println("Wrote token to spotify_token.json:\n", string(tokenJson))
		return
	}

	// use the token to get an authenticated client
	client := spotify.New(auth.Client(r.Context(), tok))
	fmt.Fprintf(w, "Login Completed!")
	ch <- client
}
