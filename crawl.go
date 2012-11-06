package main

import "encoding/json"
import "fmt"
import "io/ioutil"
import "net/http"
import "time"
import "log"
import "container/list"
import "runtime"
import "flag"

const BUS_NG_URL = "http://bus-ng.10ur.org"
const CHANNEL_BUFFER_SIZE = 64

var verbose bool

type JSONResponse struct {
    URL  string
    Body []byte
}

func FetchURL(URL string, ch chan<- *JSONResponse) {
    resp, err := http.Get(URL)
    if err != nil {
        fmt.Printf("HTTP GET FAILED for %s\n", URL)
        log.Fatal(err)
    }
    defer resp.Body.Close()

    Body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        fmt.Printf("ReadAll FAILED for %s\n", URL)
        log.Fatal(err)
    }
    ch <- &JSONResponse{URL, Body}
}

func init() {
    // http://golang.org/doc/effective_go.html#concurrency
    runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
    // counters
    var numberOfRoutes int
    var numberOfDirections int
    var numberOfTimeouts int

    // generic interface for JSON unmarshalling
    var JSONdata interface{}

    // use different channels for fun...
    agencyChannel := make(chan *JSONResponse, 1)
    routeChannel := make(chan *JSONResponse, CHANNEL_BUFFER_SIZE)
    directionChannel := make(chan *JSONResponse, CHANNEL_BUFFER_SIZE)

    // list to hold direction URLs
    directionList := list.New()

    flag.BoolVar(&verbose, "verbose", false, "Be verbose")
    flag.Parse()

    fmt.Println("--- START ---")
    go FetchURL(BUS_NG_URL+"/agencies/", agencyChannel)
    err := json.Unmarshal((<-agencyChannel).Body, &JSONdata)
    if err != nil {
        log.Fatal(err)
    }

    start := time.Now()
    for _, v := range JSONdata.(map[string]interface{}) {
        agencyTag := v.(map[string]interface{})["tag"].(string)
        go FetchURL(BUS_NG_URL+"/agencies/"+agencyTag+"/routes/", routeChannel)
        numberOfRoutes++
    }

    // 3 min. timeout for whole route crawling...
    timeout := time.After(3 * time.Minute)
    for i := 0; i < numberOfRoutes; i++ {
        select {
        case r := <-routeChannel:
            if verbose {
                fmt.Printf("Retrieved %s\n", r.URL)
            }
            err := json.Unmarshal(r.Body, &JSONdata)
            if err != nil {
                log.Fatal(err)
            }

            for _, v := range JSONdata.(map[string]interface{}) {
                routeTag := v.(map[string]interface{})["tag"].(string)
                directionList.PushBack(r.URL + routeTag + "/directions/")
            }
        case <-timeout:
            fmt.Printf("Timeout...\n")
            numberOfTimeouts++
        }
    }

    for e := directionList.Front(); e != nil; e = e.Next() {
        go FetchURL(e.Value.(string), directionChannel)
        numberOfDirections++

        // https://code.google.com/p/go/issues/detail?id=4056
        // START
        if numberOfDirections%CHANNEL_BUFFER_SIZE == 0 {
            for i := 0; i < CHANNEL_BUFFER_SIZE; i++ {
                select {
                case r := <-directionChannel:
                    if verbose {
                        fmt.Printf("\tRetrieved %s\n", r.URL)
                    }
                case <-time.After(5 * time.Second):
                    fmt.Printf("\tTimeout...\n")
                    numberOfTimeouts++
                }
            }
        }
        // END
    }

    // https://code.google.com/p/go/issues/detail?id=4056
    /*
       for i:= 0; i < numberOfDirections; i++ {
           select {
               case r := <-directionChannel:
                   fmt.Printf("\tRetrieved %s\n", r.URL)
               case <- time.After(5 * time.Second):
                   fmt.Printf("\tTimeout...\n")
           }
       }
    */

    numberOfReqs := numberOfRoutes + numberOfDirections
    elapsedTime := time.Since(start).Seconds()

    fmt.Printf("\nTook %.2f sec. and %d requests sent (~%.2f per sec / %d of them timed out) for %d routes and %d directions\n",
        elapsedTime, numberOfReqs, float64(numberOfReqs)/elapsedTime, numberOfTimeouts, numberOfRoutes, numberOfDirections)

    fmt.Println("--- END ---")
}
