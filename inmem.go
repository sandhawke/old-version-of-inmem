/*

   In-Memory implementation

*/

package pods

import (
	"fmt"
	"strings"
	"sync"
	"encoding/json"
)

type page_s struct {
	sync.RWMutex   // we're obligated to be threadsafe
	pod            *pod_s
	path           string // always starts with a slash
	contentType    string
	content        string
	lastModVersion uint64
	deleted        bool // needed for watching 404 pages, at least

	longpollQ chan chan bool   // OH, I should probably use sync.Conv instead

	/* for a dataOnly resource, the content is a JSON/RDF/whatever
	/* serialization of the appData plus some of our own metadata.
	/* For other resources, it's metadata that might be accessible
	/* somehow, eg as a nearby metadata resource. */
	dataOnly       bool
	appData        map[string]interface{}

	/* some kind of intercepter for pod_s and cluster_s to have their
	/* own special properties which appear when you call Get and Set,
	/* and which end up in the JSON somehow... */
	extraProperties func() (props []string)
    extraGetter    func(prop string) (value interface{}, handled bool)
	extraSetter    func(prop string, value interface{}) (handled bool)
}


func (page *page_s) Pod() *pod_s {
	return page.pod
}

// this is private because no one should be getting an etag separate from
// getting or setting content.   if they did, they'd likely have a race
// condition
func (page *page_s) etag() string {
	return fmt.Sprintf("%d", page.lastModVersion)
}
func (page *page_s) Path() string {
	return page.path
}
func (page *page_s) URL() string {
	return page.pod.url + page.path
}

func (page *page_s) Content(accept []string) (contentType string, content string, etag string) {
	page.RLock()
	contentType = page.contentType
	content = page.content
	etag = page.etag()
	page.RUnlock()
	return
}

// onlyIfMatch is the HTTP If-Match header value; abort if its the wrong etag
func (page *page_s) SetContent(contentType string, content string, onlyIfMatch string) (etag string, notMatched bool) {
	page.Lock()
	//fmt.Printf("onlyIfMatch=%q, etag=%q\n", onlyIfMatch, page.etag())
	if onlyIfMatch == "" || onlyIfMatch == page.etag() {
		page.contentType = contentType
		page.content = content
		page.touched() // not sure if we need to keep WLock during touched()
		etag = page.etag()
	} else {
		notMatched = true
	}
	page.Unlock()
	return
}
func (page *page_s) Delete() {
	page.Lock()
	page.deleted = true
	page.contentType = ""
	page.content = ""
	page.touched()
	page.Unlock()
}

/*
func (page *page_s) AccessControlPage() AccessController {
}
*/

func (page *page_s) touched() uint64 { // already locked
	var ch chan bool

	page.lastModVersion++

	// @@@
	// notify any selections, like _active

	for {
		select {
		case ch = <-page.longpollQ:
			ch <- true // let them know they can go on!
		default:
			return page.lastModVersion
		}
	}
}
func (page *page_s) WaitForNoneMatch(etag string) {
	page.RLock()
	// don't use defer, since we need to unlock before done
	if etag != page.etag() {
		page.RUnlock()
		return
	}
	ch := make(chan bool)
	page.longpollQ <- ch // queue up ch as a response point for us
	page.RUnlock()
	_ = <-ch // wait for that response
}

type pod_s struct {
	page_s
	url           string // which should be the same as URL(), but that's recursive
	cluster       *cluster_s
	pages         map[string]*page_s
	newPageNumber uint64
}

//func (pod *pod_s) Pages() Selection {
//}

func (pod *pod_s) Pages() (result []*page_s) {
	pod.RLock()
	result = make([]*page_s, 0, len(pod.pages))
	for _, k := range pod.pages {
		result = append(result, k)
	}
	pod.RUnlock()
	return
}

func (pod *pod_s) NewPage() (page *page_s) {
	pod.Lock()
	var path string
	for {
		path = fmt.Sprintf("/auto/%d", pod.newPageNumber)
		pod.newPageNumber++
		if _, taken := pod.pages[path]; !taken {
			break
		}
	}
	page = &page_s{}
	page.path = path
	page.pod = pod
	pod.pages[path] = page
	// pod.ALLPAGES.touched()
	pod.Unlock()
	return
}
func (pod *pod_s) PageByPath(path string, mayCreate bool) (page *page_s, created bool) {
	pod.Lock()
	page, _ = pod.pages[path]
	if mayCreate && page == nil {
		page = &page_s{}
		page.path = path
		page.pod = pod
		pod.pages[path] = page
		created = true
		// pod.ALLPAGES.touched()
	}
	if mayCreate && page.deleted {
		page.deleted = false
		created = true
		// trusting you'll set the content, and that'll trigger a TOUCHED
	}
	pod.Unlock()
	return
}
func (pod *pod_s) PageByURL(url string, mayCreate bool) (page *page_s, created bool) {
	path := url[len(pod.url)+1:]
	if path[0] != '/' {
		panic("paths must start with a slash")
	}
	return pod.PageByPath(path, mayCreate)
}

type cluster_s struct {
	page_s
	url  string // which should be the same as URL(), but that's recursive
	pods map[string]*pod_s
}

// The URL is the nominal URL of the cluster itself.  It does
// not have to be syntactically related to its pod URLs
func NewInMemoryCluster(url string) (cluster *cluster_s) {
	cluster = &cluster_s{}
	cluster.url = url
	cluster.pods = make(map[string]*pod_s)

	// and as a page?
	// leave that stuff zero for now
	return
}

func (cluster *cluster_s) Pods() (result []*pod_s) {
	cluster.RLock()
	defer cluster.RUnlock()
	result = make([]*pod_s, 0, len(cluster.pods))
	for _, k := range cluster.pods {
		result = append(result, k)
	}
	return
}

func (cluster *cluster_s) NewPod(url string) (pod *pod_s, existed bool) {
	cluster.Lock()
	defer cluster.Unlock()
	if pod, existed = cluster.pods[url]; existed {
		return
	}
	pod = &pod_s{}
	pod.cluster = cluster
	pod.url = url
	pod.pages = make(map[string]*page_s)
	cluster.pods[url] = pod
	existed = false
	// and, as a page
	pod.path = ""
	pod.pod = pod
	// leave content zero for now
	// cluster.ALLPODS.touched()
	return
}

func (cluster *cluster_s) PodByURL(url string) (pod *pod_s) {
	cluster.RLock()
	pod = cluster.pods[url]
	cluster.RUnlock()
	return
}

func (cluster *cluster_s) PageByURL(url string, mayCreate bool) (page *page_s, created bool) {
	// if we had a lot of pods we could hardcode some logic about
	// what their URLs look like, but for now this should be fine.
	cluster.RLock()
	defer cluster.RUnlock()
	for _, pod := range cluster.pods {
		if strings.HasPrefix(url, pod.url) {
			page, created = pod.PageByURL(url, mayCreate)
			return
		}
	}
	return
}



//////////////////////////////

// maybe we should generalize these as virtual properties and
// have a map of them?   But some of them are the same for every page...
//
// should we have _contentType and _content among them?
//
// that would let us serialize nonData resources in json

func (page *page_s) Properties() (result []string) {
	result = make([]string,0,len(page.appData)+5)
	result = append(result, "_id")
	result = append(result, "_etag")
	result = append(result, "_owner")
	if !page.dataOnly {
		result = append(result, "_contentType")
		result = append(result, "_content")
	}
	if page.extraProperties != nil {
		result = append(result, page.extraProperties()...)
	}
	page.RLock()
	for k := range page.appData {
		result = append(result, k)
	}
	page.RUnlock()
	return
}

func (page *page_s) Get(prop string) (value interface{}, exists bool) {
	if prop == "_id" {
		if page.pod == nil { return "", false }
		return page.URL(), true
	}
	if prop == "_etag" {  // please lock first, if using this!
		return page.etag(), true
	}
	if prop == "_owner" { 
		//if page.pod == nil { return interface{}(page).(*cluster_s).url, true }
		if page.pod == nil { return "", false }
		return page.pod.url, true
	}
	if prop == "_contentType" {
		return page.contentType, true
	}
	if prop == "_content" {
		return page.content, true
	}
	if page.extraGetter != nil {
		value, exists = page.extraGetter(prop)
		if exists {
			return value, true
		}
	}
	page.RLock()
	value, exists = page.appData[prop]
	page.RUnlock()
	return
}

func (page *page_s) Set(prop string, value interface{}) {
	if page.extraSetter != nil {
		handled := page.extraSetter(prop, value)
		if handled { return }
	}
	if prop == "_contentType" {
		page.contentType = value.(string)
		return
	}
	if prop == "_content" {
		page.content = value.(string)
		return
	}
	if prop[0] == '_' || prop[0] == '@' {
		// don't allow any (other) values to be set like this; they
		// are ours to handle.   We COULD give an error...?
		return
	}
	page.Lock()
	page.appData[prop] = value
	page.Unlock()
	return
}

func (page *page_s) MarshalJSON() (bytes []byte, err error) {
	// extra copying for simplicity for now
	props := page.Properties() 
	m := make(map[string]interface{}, len(props))
	page.RLock()
	for _, prop := range props {
		value, handled := page.Get(prop)
		if handled { m[prop] = value }
	}
	page.RUnlock()
	//fmt.Printf("Going to marshal %q for page %q, props %q", m, page, props)
	return json.Marshal(m)
	//return []byte(""), nil
}

