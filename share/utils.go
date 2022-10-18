package share

import (
	"io/ioutil"
	"reflect"
	"net/http"
	"net"
	"errors"
	"strconv"
	"math"
)

type StubMapping map[string]interface{}

func Euclidean_distance(i int, a []int, b []int) (float64) {
	var squared_sum float64 = 0
	for ; i > 0; i -= 1 {
		squared_sum += float64(a[i]) * float64(a[i]) + float64(b[i]) * float64(b[i])
	}
	return math.Sqrt(squared_sum)
}

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
    conn, err := net.Dial("udp", "8.8.8.8:80")
    if err != nil {
        ErrorLoggerPtr.Fatal(err)
    }
    defer conn.Close()

    localAddr := conn.LocalAddr().(*net.UDPAddr)

    return localAddr.IP
}

func MinOf_int32(vars ...int32) int32 {
    min := vars[0]

    for _, i := range vars {
        if min > i {
            min = i
        }
    }

    return min
}

func Call(funcName string, stub_storage StubMapping, params ... interface{}) (result interface{}, err error) {
	f := reflect.ValueOf(stub_storage[funcName])
	if len(params) != f.Type().NumIn() {
		err = errors.New("The number of params is out of index.")
		return
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		in[k] = reflect.ValueOf(param)
	}
	var res []reflect.Value
	res = f.Call(in)
	result = res[0].Interface()
	return
}

func Get_file_size(url string) (int64) {
//	url := "https://d1ohg4ss876yi2.cloudfront.net/preview/golang.png"
/*
	// we are interested in getting the file or object name
	// so take the last item from the slice
	subStringsSlice := strings.Split(url, "/")
	fileName := subStringsSlice[len(subStringsSlice)-1]
*/
	resp, err := http.Head(url)
	if err != nil {
		ErrorLoggerPtr.Fatal("Error getting header:", err)
	}

	// Is our request ok?

	if resp.StatusCode != http.StatusOK {
		ErrorLoggerPtr.Fatal("HTTP error:", resp.Status)
		// exit if not ok
	}

	// the Header "Content-Length" will let us know
	// the total file size to download
	size, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	downloadSize := int64(size)

	return downloadSize

 }

 func Http_download(resource string, begin int64, end int64) (*[]byte) {
	 req, _ := http.NewRequest("GET", resource, nil)
	 req.Header.Add("Range", "bytes=" +  strconv.FormatInt(begin, 10) + "-" + strconv.FormatInt(end, 10))
	 //fmt.Println(req)
	 var client http.Client
	 resp, _ := client.Do(req) // TODO download to local file
	 //fmt.Println(resp)
	 body, _ := ioutil.ReadAll(resp.Body) // TODO check the error
	 //fmt.Println(len(body))
	 return &body
 }
