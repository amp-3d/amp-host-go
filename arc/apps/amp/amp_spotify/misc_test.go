package amp_spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"

	"github.com/zmb3/spotify/v2"
)

var (
	ch    = make(chan *spotify.Client)
	state = "abc123"
)

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

	url := gAuth.AuthURL(state)
	fmt.Println("Please log in to Spotify by visiting the following page in your browser:", url)

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
	tok, err := gAuth.Token(r.Context(), state, r)
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
	client := spotify.New(gAuth.Client(r.Context(), tok))
	fmt.Fprintf(w, "Login Completed!")
	ch <- client
}

