package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

var pod, container, call string

func id() string {
	return fmt.Sprintf("I am pod:%s container:%s", pod, container)
}

func logId() {
	t := time.NewTicker(time.Second)
	go func() {
		for {
			<-t.C
			log.Printf("%d %s\n", time.Now().Unix(), id())
		}
	}()
}

func callRemote(w http.ResponseWriter, _ *http.Request) {
	resp, err := http.Get(call)
	if err != nil {
		log.Println(err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return
	}
	w.Write([]byte(fmt.Sprintf("Remote response: %s", string(body))))

}

func config(w http.ResponseWriter, _ *http.Request) {
	dat, err := ioutil.ReadFile("/etc/app-config/file.conf")
	if err != nil {
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(dat)
}

func filesystem(w http.ResponseWriter, r *http.Request) {
	var content []string
	for k, v := range r.URL.Query() {
		file := fmt.Sprintf("/tmp/%s", k)
		if v[0] != "" {
			err := ioutil.WriteFile(file, []byte(v[0]), 0644)
			if err != nil {
				log.Println(err)
			}
		}
		if dat, err := ioutil.ReadFile(file); err != nil {
			content = append(content, fmt.Sprintf("%s=%s", file, err.Error()))
		} else {
			content = append(content, fmt.Sprintf("%s=%s", file, dat))
		}
	}
	w.Write([]byte(strings.Join(content, "\n")))
}

func hello(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte(id()))
}

func main() {
	bind := flag.String("listen", ":8080", "Address to listen on")
	flag.StringVar(&call, "call", "http://localhost:8081", "Address to call when using 'call-remote'")
	flag.StringVar(&pod, "pod", "", "Name of this pod")
	flag.StringVar(&container, "container", "", "Name of container in the pod")
	flag.Parse()
	logId()
	http.HandleFunc("/cfg", config)
	http.HandleFunc("/fs", filesystem)
	http.HandleFunc("/call", callRemote)
	http.HandleFunc("/", hello)

	log.Printf("Starting on %s\n", *bind)
	err := http.ListenAndServe(*bind, nil)
	if err != http.ErrServerClosed {
		log.Println(err)
	}
	log.Println("bye")
}
