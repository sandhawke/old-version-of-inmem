package pods

import (
	"testing"
	//"fmt"
	"time"
	"strconv"
	"sync"
)

func Test0(t *testing.T) {
	//var c *Cluster
	//var p1 *Pod
	//var g1 *Page

	c := NewInMemoryCluster("http://cluster.example")
	p1, p1x := c.NewPod("http://pod1.example")
	if p1x { t.Fail() }
	if p1 == nil { t.Fail() }
	g1 := p1.NewPage()
	if g1.URL() != "http://pod1.example/auto/0" { t.Fail() }
	g2 := p1.NewPage()
	if g2.URL() != "http://pod1.example/auto/1" { t.Fail() }
	g2.Delete()
	g3 := p1.NewPage()
	if g3.URL() != "http://pod1.example/auto/2" { t.Fail() }
}


/*

    Run a bunch of goroutines each trying to increment a number stored
    as a string in a page, using etags to handle concurrency.
    Actually works....

*/
func incrementer(page Page,times int,sleep time.Duration,wg *sync.WaitGroup,id int) {
	//fmt.Printf("incr started\n");
	for i:=0; i<times; {
		_,content,etag := page.Content([]string{})
		n,_ := strconv.ParseUint(content, 10, 64)
		n++
		content = strconv.FormatUint(n, 10)
		time.Sleep(sleep)
		_, notMatched := page.SetContent("", content, etag)
		if notMatched {
			// fmt.Printf("was beaten to %d\n", n);
		} else {
			//fmt.Printf("%d did incr to %d\n", id, n);
			i++
		}
	}
	wg.Done()
	//fmt.Printf("done\n");
}

func TestIncr1(t *testing.T) {
	c := NewInMemoryCluster("http://cluster.example")
	pod,_ := c.NewPod("http://pod1.example")
	page := pod.NewPage()
	_,_ = page.SetContent("", "1000", "")
	var wg sync.WaitGroup
	for i:=0; i<10; i++ {
		wg.Add(1)
		go incrementer(page,10,5*time.Millisecond,&wg,i)
	}
	wg.Wait()
	_,content,_ := page.Content([]string{})
	//fmt.Printf(content)
	if content != "1100" { t.Fail() }
}
