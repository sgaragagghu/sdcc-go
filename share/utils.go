package share

import (
	"io/ioutil"
	"reflect"
	"net/http"
	"net"
	"errors"
	"strconv"
	"math"
	"fmt"
)

type StubMapping map[string]interface{}

func Welford_one_pass(mean []float64, sample []float64, nsamples float64) ([]float64) {
	if(nsamples > 0) {
		for i, _ := range mean {
			//InfoLoggerPtr.Println("before mean", mean, "sample", sample, "nsamples", nsamples)
			mean[i] = mean[i] + (sample[i] - mean[i]) / nsamples
			//InfoLoggerPtr.Println("after", mean)
		}
	}
	return mean
}

func Euclidean_distance(i int, a []float64, b []float64) (float64) {
	var squared_sum float64 = 0
	for i -= 1; i >= 0; i -= 1 {
		squared_sum += math.Pow(a[i] - b[i], 2)
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

// TODO check generics
func MinOf_int32(vars ...int32) int32 {
    min := vars[0]

    for _, i := range vars {
        if min > i {
            min = i
        }
    }

    return min
}

func compatible(actual, expected reflect.Type) bool {
	if actual == nil {
		k := expected.Kind()
		return k == reflect.Chan ||
			k == reflect.Func ||
			k == reflect.Interface ||
			k == reflect.Map ||
			k == reflect.Ptr ||
			k == reflect.Slice
	}
	return actual.AssignableTo(expected)
}

func Call(funcName string, stub_storage StubMapping, params ... interface{}) (result interface{}, err error) {
	f := reflect.ValueOf(stub_storage[funcName])
	funcType := reflect.TypeOf(stub_storage[funcName])
	if len(params) != f.Type().NumIn() {
		err = errors.New("The number of params is out of index.")
		return
	}
	in := make([]reflect.Value, len(params))
	for k, param := range params {
		expectedType := funcType.In(k)
		actualType := reflect.TypeOf(param)

		if !compatible(actualType, expectedType) {
			err = fmt.Errorf("InvocationCausedPanic called with a mismatched parameter type [parameter #%v: expected %v; got %v].", k, expectedType, actualType)
			return
		}

		if param == nil {
			in[k] = reflect.New(expectedType).Elem()
		} else {
			in[k] = reflect.ValueOf(param)
		}
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
