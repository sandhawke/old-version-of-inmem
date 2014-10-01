/*

I CANT ACTUALLY GET THIS TO WORK.  IT LOOKS LIKE type alignement isn't recursive or something.   

:-(

https://code.google.com/p/go-wiki/wiki/MethodSets

Maybe we don't need it, though.   

Can I still do a logging wrapper?
Implement each of these and pass it through?
How about JSON data access?


   This feels a bit clumsy in places, but that's in order to make it
   threadsafe.  Everything should use RWMutex or something equivalent
   internally.

   I'm writing this while thinking of an in-memory implementation in
   the server, but it could also use a backing database, and/or this
   could actually be client-side, talking http to a server, I think.

   The key concept is that a Pod is a set of Pages owned by a single
   users.  Pods are grouped together into a Cluster, which might be a
   server or set of servers.

*/

package pods

type Page interface {
	//Pod() *Pod
	Path() string
	URL() string
	Content(accept []string) (contentType string, content string, etag string)
	//AccessControlPage() AccessController
	SetContent(contentType string, content string, onlyIfMatch string) (etag string, notMatched bool)
	WaitForNoneMatch(etag string)
}

/*
type DataPage interface {
	Page
	Get(prop string) interface{} // but what are those if maps, arrays?
	Set(prop string, value interface{})
	// Copy()    map[string]interface{}
	// Replace(map)
	// Vocab() vocab.Vocab
	// RDF access stuff
}

type ExternalUserId string

type AccessController interface {
	DataPage
	ReadableBy(indiv ExternalUserId) bool
	SetReadableBy(indiv ExternalUserId)
	ClearReadableBy(indiv ExternalUserId)
	Readers() []ExternalUserId
}

type Selection interface {
	DataPage
	//
	// HAS MEMBERS...
	//
	// What should we do about them?
}
*/

type Pod interface {
	Page
	Pages() []*Page
	NewPage() (page *Page)
	PageByPath(path string, myCreate bool) (page *Page, created bool)
	PageByURL(path string, myCreate bool) (page *Page, created bool)
}

type Cluster interface {
	Page
	Pods() []*Pod
	NewPod(url string) (pod *Pod, existed bool)
	PodByURL(url string) (pod *Pod)
	PageByURL(url string, mayCreate bool) (page *Page, created bool)
}
