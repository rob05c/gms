package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rob05c/gms/gms"

	groveweb "github.com/apache/trafficcontrol/grove/web"
)

func main() {
	port := flag.Int("port", 80, "the port to serve on")
	maxHistory := flag.Int("maxHistory", 10, "the max mutate history to retain")
	mutateInterval := flag.Duration("mutateInterval", time.Second, "the interval to randomly mutate the object")
	flag.Parse()
	http.HandleFunc("/", GetHandler(*maxHistory, *mutateInterval))
	fmt.Printf("Serving MutateInterval %v, MaxHistory %d on %d\n", *mutateInterval, *maxHistory, *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

const HeaderGetModifiedSince = "Get-Modified-Since"

func GetHandler(maxHistory int, mutateInterval time.Duration) http.HandlerFunc {
	obj := gms.NewThsObj()
	objHist := gms.NewThsObjs(maxHistory)
	go ObjMutator(obj, objHist, mutateInterval)

	return func(w http.ResponseWriter, req *http.Request) {

		gmsTime := (*time.Time)(nil)
		if gmsHeader := req.Header.Get(HeaderGetModifiedSince); gmsHeader != "" {
			if isETag := gmsHeader[0] == '"' && gmsHeader[len(gmsHeader)-1] == '"'; isETag {
				eTag := gmsHeader[1 : len(gmsHeader)-1]
				fmt.Println("Get-Modified-Since Header '" + gmsHeader + "' is ETag '" + eTag + "'")
				if gmsHeaderTime, err := gms.ParseETag(eTag); err == nil {
					gmsTime = &gmsHeaderTime
					fmt.Println("Get-Modified-Since got ETag: " + gmsTime.Format(time.RFC3339Nano))
				} else {
					fmt.Println("Get-Modified-Since Header '" + gmsHeader + "' ETag malformed; ignoring: " + err.Error())
				}
			} else {
				if gmsHeaderTime, ok := groveweb.ParseHTTPDate(gmsHeader); ok {
					gmsTime = &gmsHeaderTime
				} else {
					fmt.Println("Get-Modified-Since Header '" + gmsHeader + "' not quoted (ETag) and not a HTTP-date; ignoring")
				}
			}
		}

		if gmsTime == nil {
			fmt.Println("Client requested without Get-Modified-Since, returning whole object")
			latestObj := obj.Get()
			fmt.Printf("sending %+v\n", latestObj)

			bts, err := json.Marshal(latestObj.O)
			if err != nil {
				fmt.Println("Error marshalling obj: " + err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			et := gms.GenerateETag(latestObj.T)
			fmt.Println("Setting ETag: " + et)
			w.Header().Set("ETag", gms.GenerateETag(latestObj.T))
			w.Header().Set("Content-Type", gms.MimeTypeJSON)
			w.Write(bts)
			return
		}

		latestObj := obj.Get()
		fmt.Printf("lastTime: %v gmsTime %v\n", latestObj.T, *gmsTime)
		if latestObj.T.Before(*gmsTime) {
			fmt.Println("Client requested Get-Modified-Since, but unchanged, returning empty patch")
			// If the time hasn't changed, return an empty patch. Clients should usually send an If-Modified-Since so this doesn't happen.
			bts, err := json.Marshal([]gms.JSONPatchOp{})
			if err != nil {
				fmt.Println("Error marshalling empty patch obj: " + err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", gms.MimeTypeJSONPatch)
			w.Header().Set("ETag", gms.GenerateETag(latestObj.T))
			w.Write(bts)
			return
		}

		notNewerThanObj := objHist.GetNotNewerThan(*gmsTime)
		if notNewerThanObj.T.After(*gmsTime) {
			fmt.Println("Client requested Get-Modified-Since older than history, returning whole object")
			latestObj := obj.Get()
			fmt.Printf("sending %+v\n", latestObj)
			bts, err := json.Marshal(latestObj.O)
			if err != nil {
				fmt.Println("Error marshalling obj: " + err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("ETag", gms.GenerateETag(latestObj.T))
			w.Header().Set("Content-Type", gms.MimeTypeJSON)
			w.Write(bts)
			return
		}

		fmt.Println("Client requested Get-Modified-Since, returning patch")
		patch := gms.CreatePatch(notNewerThanObj.O, latestObj.O)
		bts, err := json.Marshal(patch)
		if err != nil {
			fmt.Println("Error marshalling patch obj: " + err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("ETag", gms.GenerateETag(latestObj.T))
		w.Header().Set("Content-Type", gms.MimeTypeJSONPatch)
		w.Write(bts)
		return
	}
}

// ObjMutator periodically mutates the given ThsObj. It does not return; it is designed to be called in a goroutine.
func ObjMutator(thsObj *gms.ThsObj, objHist *gms.ThsObjs, interval time.Duration) {
	c := time.Tick(interval)
	for range c {
		o := thsObj.Get()
		o.O = o.O.RandMutate()
		o.T = time.Now()
		objHist.Add(o.O)
		thsObj.Set(o)
	}
}
