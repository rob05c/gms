package gms

import (
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

const HeaderGetModifiedSince = "Get-Modified-Since"

const HeaderIfNoneMatch = "If-None-Match"
const HeaderAcceptInstanceManipulation = "A-IM"
const HeaderInstanceManipulation = "IM"
const HeaderContentType = "Content-Type"
const HeaderETag = "ETag"
const HeaderDeltaBase = "Delta-Base"

const InstanceManipulationValueJSONPatch = "jsonpatch"
const InstanceManipulationValueGzip = "gzip"

const MimeTypeJSONPatch = "application/json-patch+json"
const MimeTypeJSON = "application/json"

type Obj struct {
	FooA Foo `json:"foo-a"`
	FooB Foo `json:"foo-b"`
}

type Foo struct {
	BarA Bar `json:"bar-a"`
	BarB Bar `json:"bar-b"`
}

type Bar struct {
	BazA int64 `json:"baz-a"`
	BazB int64 `json:"baz-b"`
}

// ThsObj is a threadsafe Obj
type ThsObj struct {
	o *ObjTime
	m *sync.Mutex
}

func NewThsObj() *ThsObj {
	return &ThsObj{o: &ObjTime{T: time.Time{}}, m: &sync.Mutex{}}
}

func (o *ThsObj) Get() ObjTime {
	o.m.Lock()
	defer o.m.Unlock()
	return *o.o
}

func (o *ThsObj) Set(newO ObjTime) {
	o.m.Lock()
	defer o.m.Unlock()
	o.o = &newO
}

type ObjTime struct {
	T time.Time
	O Obj
}

// ThsObjs is a threadsafe slice of Objs
type ThsObjs struct {
	o   []ObjTime
	m   sync.Mutex
	max int
}

func NewThsObjs(maxHistory int) *ThsObjs {
	return &ThsObjs{max: maxHistory}
}

func (o *ThsObjs) Add(newO Obj) {
	o.m.Lock()
	defer o.m.Unlock()
	o.o = append([]ObjTime{ObjTime{T: time.Now(), O: newO}}, o.o...)
	if len(o.o) > o.max {
		o.o = o.o[:o.max]
	}
}

// GetNotNewerThan returns the newest object not newer than the given time. This is designed to be used to generate a patch, when a client has an object they got at a certain time, this allows getting the object at least as old as they have, and then generate the patch changes for the current new object, diffing their old one.
// If t is older than the first object, the oldest object is returned.
// If o has no objects, a default object is returned.
func (o *ThsObjs) GetNotNewerThan(t time.Time) ObjTime {
	o.m.Lock()
	defer o.m.Unlock()
	os := o.o
	for _, o := range os {
		if o.T.After(t) {
			fmt.Printf("GetNotNewerThan %v after %v, skipping\n", o.T, t)
			continue
		}
		fmt.Printf("GetNotNewerThan %v before %v, returning\n", o.T, t)
		return o
	}
	if len(os) == 0 {
		fmt.Printf("GetNotNewerThan has nothing returning {}\n")
		return ObjTime{}
	}
	fmt.Printf("GetNotNewerThan has nothing before %v, returning oldest\n", t)
	return os[len(os)-1] // return oldest - the requested date is older than the oldest
}

const JSONPatchOpReplace = "replace"

type JSONPatchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// CreatePatch creates a JSON Patch of the changes in b which are different from a.
func CreatePatch(a, b Obj) []JSONPatchOp {
	patches := []JSONPatchOp{}
	if a.FooA.BarA.BazA != b.FooA.BarA.BazA {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-a/bar-a/baz-a", Value: b.FooA.BarA.BazA})
	}
	if a.FooA.BarA.BazB != b.FooA.BarA.BazB {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-a/bar-a/baz-b", Value: b.FooA.BarA.BazB})
	}
	if a.FooA.BarB.BazA != b.FooA.BarB.BazA {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-a/bar-b/baz-a", Value: b.FooA.BarB.BazA})
	}
	if a.FooA.BarB.BazB != b.FooA.BarB.BazB {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-a/bar-b/baz-b", Value: b.FooA.BarB.BazB})
	}
	if a.FooB.BarA.BazA != b.FooB.BarA.BazA {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-b/bar-a/baz-a", Value: b.FooB.BarA.BazA})
	}
	if a.FooB.BarA.BazB != b.FooB.BarA.BazB {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-b/bar-a/baz-b", Value: b.FooB.BarA.BazB})
	}
	if a.FooB.BarB.BazA != b.FooB.BarB.BazA {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-b/bar-b/baz-a", Value: b.FooB.BarB.BazA})
	}
	if a.FooB.BarB.BazB != b.FooB.BarB.BazB {
		patches = append(patches, JSONPatchOp{Op: JSONPatchOpReplace, Path: "/foo-b/bar-b/baz-b", Value: b.FooB.BarB.BazB})
	}
	return patches
}

// AppyPatch applies the given patches to the given object.
// Note it only supports a full path, setting the Baz objects, and replace operations, as returned by CreatePatch. Partial paths or other operations will return an error.
func ApplyPatch(obj Obj, patches []JSONPatchOp) (Obj, error) {
	for _, patch := range patches {
		val, ok := patch.Value.(float64)
		if !ok {
			return Obj{}, fmt.Errorf("unsupported value for path '"+patch.Path+"': %T", patch.Value)
		}

		path := strings.Split(patch.Path, "/")
		if len(path) != 4 {
			return Obj{}, fmt.Errorf("unsupported patch path for Obj, expected 4 parts: '%v' got '%+v'", patch.Path, path)
		}
		path = path[1:] // remove initial empty /
		if path[0] == "foo-a" {
			if path[1] == "bar-a" {
				if path[2] == "baz-a" {
					obj.FooA.BarA.BazA = int64(val)
				} else if path[2] == "baz-b" {
					obj.FooA.BarA.BazB = int64(val)
				} else {
					return Obj{}, errors.New("unsupported patch path for Obj, unsupported part: '" + path[2] + "' in '" + patch.Path + "'")
				}
			} else if path[1] == "bar-b" {
				if path[2] == "baz-a" {
					obj.FooA.BarB.BazA = int64(val)
				} else if path[2] == "baz-b" {
					obj.FooA.BarB.BazB = int64(val)
				} else {
					return Obj{}, errors.New("unsupported patch path for Obj, unsupported part: '" + path[2] + "' in '" + patch.Path + "'")
				}
			} else {
				return Obj{}, errors.New("unsupported patch path for Obj, unsupported part: '" + path[1] + "' in '" + patch.Path + "'")
			}
		} else if path[0] == "foo-b" {
			if path[1] == "bar-a" {
				if path[2] == "baz-a" {
					obj.FooB.BarA.BazA = int64(val)
				} else if path[2] == "baz-b" {
					obj.FooB.BarA.BazB = int64(val)
				} else {
					return Obj{}, errors.New("unsupported patch path for Obj, unsupported part: '" + path[2] + "' in '" + patch.Path + "'")
				}
			} else if path[1] == "bar-b" {
				if path[2] == "baz-a" {
					obj.FooB.BarB.BazA = int64(val)
				} else if path[2] == "baz-b" {
					obj.FooB.BarB.BazB = int64(val)
				} else {
					return Obj{}, errors.New("unsupported patch path for Obj, unsupported part: '" + path[2] + "' in '" + patch.Path + "'")
				}
			} else {
				return Obj{}, errors.New("unsupported patch path for Obj, unsupported part: '" + path[1] + "' in '" + patch.Path + "'")
			}
		} else {
			return Obj{}, errors.New("unsupported patch path for Obj, unsupported part: '" + path[0] + "' in '" + patch.Path + "'")
		}
	}
	return obj, nil
}

// RandMutate randomly changes the given object, and returns the new changed object.
func (o Obj) RandMutate() Obj {
	foo := &o.FooA
	if rand.Intn(2) == 0 {
		foo = &o.FooB
	}

	bar := &foo.BarA
	if rand.Intn(2) == 0 {
		bar = &foo.BarB
	}

	baz := &bar.BazA
	if rand.Intn(2) == 0 {
		baz = &bar.BazB
	}
	*baz = *baz + 1

	return o
}

func GenerateETag(t time.Time) string {
	return strconv.FormatInt(t.UnixNano(), 10)
}

func ParseETag(eTag string) (time.Time, error) {
	i, err := strconv.ParseInt(eTag, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	fmt.Printf("ParseEtag num %+v\n", i)
	t := time.Unix(0, i)
	fmt.Printf("ParseEtag time %+v\n", t)
	return t, nil
}

// ThsObjETag is a threadsafe Obj with an ETag
type ThsObjETag struct {
	o Obj
	e string
	m sync.Mutex
}

func NewThsObjETag() *ThsObjETag {
	return &ThsObjETag{}
}

func (o *ThsObjETag) Get() (Obj, string) {
	o.m.Lock()
	defer o.m.Unlock()
	return o.o, o.e
}

func (o *ThsObjETag) Set(newO Obj, newETag string) {
	o.m.Lock()
	defer o.m.Unlock()
	o.o = newO
	o.e = newETag
}
