/*

   In-Memory implementation

*/

package pods

import (
	"fmt"
	"strings"
	"sync"
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
	result = make([]*page_s, len(pod.pages))
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
	result = make([]*pod_s, len(cluster.pods))
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
