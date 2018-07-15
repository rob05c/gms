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
			if gmsHeaderTime, ok := groveweb.ParseHTTPDate(gmsHeader); ok {
				gmsTime = &gmsHeaderTime
			} else {
				fmt.Println("Get-Modified-Since Header '" + gmsHeader + "' not a HTTP-date; ignoring")
			}
		}
		if gmsTime != nil {
			fmt.Printf("lastTime: %v gmsTime %v\n", obj.Get().T, *gmsTime)
			if obj.Get().T.Before(*gmsTime) {
				fmt.Println("Client requested Get-Modified-Since, but unchanged, returning empty patch")
				// If the time hasn't changed, return an empty patch. Clients should usually send an If-Modified-Since so this doesn't happen.
				bts, err := json.Marshal([]gms.JSONPatchOp{})
				if err != nil {
					fmt.Println("Error marshalling empty patch obj: " + err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", gms.MimeTypeJSONPatch)
				w.Write(bts)
			}

			o := objHist.GetNotNewerThan(*gmsTime)
			if o.T.After(*gmsTime) {
				fmt.Println("Client requested Get-Modified-Since older than history, returning whole object")
				debugO := obj.Get()
				fmt.Printf("sending %+v\n", debugO)

				bts, err := json.Marshal(obj.Get().O)
				if err != nil {
					fmt.Println("Error marshalling obj: " + err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", gms.MimeTypeJSON)
				w.Write(bts)
			} else {
				fmt.Println("Client requested Get-Modified-Since, returning patch")
				patch := gms.CreatePatch(o.O, obj.Get().O)
				bts, err := json.Marshal(patch)
				if err != nil {
					fmt.Println("Error marshalling patch obj: " + err.Error())
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", gms.MimeTypeJSONPatch)
				w.Write(bts)
			}
		} else {
			fmt.Println("Client requested without Get-Modified-Since, returning whole object")
			w.Header().Set("Content-Type", gms.MimeTypeJSON)

			debugO := obj.Get()
			fmt.Printf("sending %+v\n", debugO)

			bts, err := json.Marshal(obj.Get().O)
			if err != nil {
				fmt.Println("Error marshalling obj: " + err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write(bts)
		}
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
