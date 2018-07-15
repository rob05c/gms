package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	groveweb "github.com/apache/trafficcontrol/grove/web"
	"github.com/rob05c/gms/gms"
)

func main() {
	server := flag.String("server", "http://localhost", "the server URI to poll for object changes, including the scheme")
	pollInterval := flag.Duration("pollInterval", time.Second, "the interval to poll the server")
	flag.Parse()

	fmt.Printf("Client server '%v' pollInterval %v starting\n", *server, *pollInterval)

	obj := gms.NewThsObj()
	log.Fatal(ServerPoller(obj, *server, *pollInterval))
}

// ServerPoller periodically polls the server and updates the Obj. It does not return, unless there is an error; it is designed to be called in a goroutine.
func ServerPoller(obj *gms.ThsObj, serverURI string, interval time.Duration) error {
	c := time.Tick(interval)
	for range c {
		if err := PollServer(obj, serverURI); err != nil {
			return errors.New("polling server: " + err.Error())
		}

		bts, err := json.Marshal(obj.Get())
		if err != nil {
			return errors.New("marshalling object: " + err.Error())
		}
		fmt.Println("Got: " + string(bts))
	}
	return nil
}

func ToHTTPDate(t time.Time) string { return t.Format(time.RFC1123) }

// PollServer updates the given obj from the given server URI.
func PollServer(obj *gms.ThsObj, serverURI string) error {
	client := &http.Client{}

	req, err := http.NewRequest(http.MethodGet, serverURI, nil)
	if err != nil {
		return errors.New("creating request: " + err.Error())
	}

	lastObj := obj.Get()
	lastObjTime := lastObj.T
	defaultTime := time.Time{}
	if lastObjTime != defaultTime {
		fmt.Println("Adding Request Get-Modified-Since Header: " + ToHTTPDate(lastObjTime))
		req.Header.Add("Get-Modified-Since", ToHTTPDate(lastObjTime))
	} else {
		fmt.Println("Not Adding Request Get-Modified-Since Header")
	}

	resp, err := client.Do(req)
	if err != nil {
		return errors.New("requesting server '" + serverURI + "': " + err.Error())
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	contentType = strings.ToLower(contentType)
	contentType = strings.Replace(contentType, " ", "", -1)
	newObj := gms.Obj{}
	if contentType == gms.MimeTypeJSONPatch {
		patches := []gms.JSONPatchOp{}
		if err := json.NewDecoder(resp.Body).Decode(&patches); err != nil {
			return errors.New("decoding patch response '" + serverURI + "': " + err.Error())
		}

		patchJSON, err := json.Marshal(patches)
		if err != nil {
			return fmt.Errorf("marshalling patch response '%+v': %v", patches, err)
		}
		objJSON, err := json.Marshal(lastObj)
		if err != nil {
			return fmt.Errorf("marshalling object '%+v': %v", patches, err)
		}
		fmt.Println("Got Patch: " + string(patchJSON))
		fmt.Println("Applying Patch To: " + string(objJSON))

		if newObj, err = gms.ApplyPatch(lastObj.O, patches); err != nil {
			return fmt.Errorf("applying patch response '%+v': %v", patches, err)
		}
	} else {
		fmt.Println("Decoding Non-Patch")
		if err := json.NewDecoder(resp.Body).Decode(&newObj); err != nil {
			return errors.New("decoding response '" + serverURI + "': " + err.Error())
		}
	}
	date := resp.Header.Get("Date")
	t, ok := groveweb.ParseHTTPDate(date)
	if !ok {
		return errors.New("decoding response: invalid date: " + date)
	}
	obj.Set(gms.ObjTime{O: newObj, T: t})

	return nil
}
