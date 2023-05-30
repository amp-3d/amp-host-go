package amp_bcat

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arcspace/go-archost/arc/apps/amp_family/amp"
)

func TestLogin(t *testing.T) {

	tr := &http.Transport{
		MaxIdleConns:       10,
		IdleConnTimeout:    5 * time.Second,
		DisableCompression: true,
	}

	client := &http.Client{
		Transport: tr,
	}

	const jsonStream = `{
	"username": "DrewZ",
	"password": "trdtrtvrtretttetrbrtbertb"
	}`
	// 	"create": "Mnp*845}sbk"

	resp, err := client.Post("https://api.soundspectrum.com/v1/login_create/", "application/json", strings.NewReader(jsonStream))
	if err != nil {
		log.Fatal(err)
	}

	{
		var pb amp.LoginCreateResponse
		dec := json.NewDecoder(resp.Body)
		err := dec.Decode(&pb)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%v\n", pb)
	}

}
