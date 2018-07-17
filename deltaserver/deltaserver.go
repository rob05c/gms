package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rob05c/gms/gms"
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

func StrSliceToMap(ss []string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

// ETagTimes returns a map of the given etags to the times they represent.
// Does not return an error if any are invalid, but simply omits invalid etags from the returned map. This behavior is typically desired, because we typically ignore invalid etags, and continue to use valid ones.
func ETagTimes(etags []string) map[string]time.Time {
	ts := map[string]time.Time{}
	for _, etag := range etags {
		if len(etag) < 2 {
			continue // etags must be quoted
		}
		etag = etag[1 : len(etag)-1] // strip quotes
		etagT, err := gms.ParseETag(etag)
		if err != nil {
			continue
		}
		ts[etag] = etagT
	}
	fmt.Printf("ETagTimes %v -> %v\n", etags, ts)
	return ts
}

// LatestETag returns the latest etag in the map, its time, and whether the map was empty.
func LatestETag(etags map[string]time.Time) (string, time.Time, bool) {
	latestETag := ""
	latestTime := time.Time{}
	found := false
	for etag, t := range etags {
		if t.After(latestTime) {
			latestETag = etag
			latestTime = t
			found = true
		}
	}
	return latestETag, latestTime, found
}

func GetHandler(maxHistory int, mutateInterval time.Duration) http.HandlerFunc {
	obj := gms.NewThsObj()
	objHist := gms.NewThsObjs(maxHistory)
	go ObjMutator(obj, objHist, mutateInterval)

	return func(w http.ResponseWriter, req *http.Request) {
		fmt.Printf("DEBUG header AIM %v INM %v\n", req.Header.Get(gms.HeaderAcceptInstanceManipulation), req.Header.Get(gms.HeaderIfNoneMatch))
		etag, etagTime, etagFound := "", time.Time{}, false
		aimVals := StrSliceToMap(strings.Split(req.Header.Get(gms.HeaderAcceptInstanceManipulation), ","))
		if _, ok := aimVals[gms.InstanceManipulationValueJSONPatch]; ok {
			etag, etagTime, etagFound = LatestETag(ETagTimes(strings.Split(req.Header.Get(gms.HeaderIfNoneMatch), ",")))
		}

		if !etagFound {
			fmt.Println("Client requested without A-IM and If-None-Match with valid ETags, returning whole object")
			latestObj := obj.Get()
			fmt.Printf("sending %+v\n", latestObj)

			bts, err := json.Marshal(latestObj.O)
			if err != nil {
				fmt.Println("Error marshalling obj: " + err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			et := gms.GenerateETag(latestObj.T)
			fmt.Printf("Setting ETag '%v' from '%v'", et, latestObj.T)
			w.Header().Set(gms.HeaderETag, gms.GenerateETag(latestObj.T))
			w.Header().Set(gms.HeaderContentType, gms.MimeTypeJSON)
			w.Write(bts)
			return
		}

		latestObj := obj.Get()
		fmt.Printf("lastTime: %v latest etagTime %v\n", latestObj.T, etagTime)
		if !latestObj.T.After(etagTime) {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		notNewerThanObj := objHist.GetNotNewerThan(etagTime)
		if notNewerThanObj.T.After(etagTime) {
			fmt.Println("Client requested A-IM older than history, returning whole object")
			latestObj := obj.Get()
			fmt.Printf("sending %+v\n", latestObj)
			bts, err := json.Marshal(latestObj.O)
			if err != nil {
				fmt.Println("Error marshalling obj: " + err.Error())
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set(gms.HeaderETag, gms.GenerateETag(latestObj.T))
			w.Header().Set(gms.HeaderContentType, gms.MimeTypeJSON)
			w.Write(bts)
			return
		}

		fmt.Println("Client requested A-IM, returning patch")
		patch := gms.CreatePatch(notNewerThanObj.O, latestObj.O)
		bts, err := json.Marshal(patch)
		if err != nil {
			fmt.Println("Error marshalling patch obj: " + err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set(gms.HeaderETag, gms.GenerateETag(latestObj.T))
		w.Header().Set(gms.HeaderContentType, gms.MimeTypeJSONPatch)
		w.Header().Set(gms.HeaderDeltaBase, `"`+etag+`"`)
		w.Header().Set(gms.HeaderInstanceManipulation, gms.InstanceManipulationValueJSONPatch)
		w.WriteHeader(http.StatusIMUsed)
		w.Write(bts)
		return
	}
}

// ObjMutator periodically mutates the given ThsObj. It does not return; it is designed to be called in a goroutine.
func ObjMutator(thsObj *gms.ThsObj, objHist *gms.ThsObjs, interval time.Duration) {
	o := thsObj.Get()
	o.O = o.O.RandMutate()
	o.T = time.Now()
	objHist.Add(o.O)
	thsObj.Set(o)

	c := time.Tick(interval)
	for range c {
		o := thsObj.Get()
		o.O = o.O.RandMutate()
		o.T = time.Now()
		objHist.Add(o.O)
		thsObj.Set(o)
	}
}
