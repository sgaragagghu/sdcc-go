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
	"bufio"
	"bytes"
	"unsafe"
)

type StubMapping map[string]interface{}

var MaxInt int64

const BYTE_SIZE = 8

func init() {
	// Retrieving max int number, for checking overflows
	var try int = 0
	bit := unsafe.Sizeof(try) * BYTE_SIZE
	MaxInt =  1<<(bit - 1) - 1
}

func Check_float64_to_int_overflow(a float64) (bool) {

	//fmt.Println("maxint", MaxInt, "maxint64",math.MaxInt64, "maxint32", math.MaxInt32)

	if MaxInt == math.MaxInt64 {
		if a > math.MaxInt64 { return true }
	} else if MaxInt == math.MaxInt32 {
		if a > math.MaxInt32 { return true }
	} else {
		WarningLoggerPtr.Println("Unexpected error.")
	}
	return false
}
// Get first n (count) words of a string)
func First_words(value string, count int) string {
	// Loop over all indexes in the string.
	for i := range value {
		// If we encounter a space, reduce the count.
		if value[i] == ' ' {
			count -= 1
			// When no more words required, return a substring.
			if count == 0 {
				return value[0:i]
			}
		}
	}
	// Return the entire string.
	return value
}
// Just simple parser forr comma separated values kind of files
func Parser_simple(point *[]float64, buffered_read *bufio.Reader, separate_properties byte, separate_entries byte) (full_s string, err error) {

	s := ""
	j := 1
	var char byte
	for char, err = buffered_read.ReadByte(); err == nil; char, err = buffered_read.ReadByte() {
		//InfoLoggerPtr.Println(string(char))
		if char == separate_properties { // found a property
			if j < len(*point)  {
				(*point)[j - 1], err = strconv.ParseFloat(s, 64)
				if err != nil {
					WarningLoggerPtr.Println("overflow!")
					err = nil
				}
				full_s = s + string(separate_properties)
				s = "" //resetting the string since we have to append the next property value
				j += 1
			} else { ErrorLoggerPtr.Fatal("Parsing failed") }
		} else if char == separate_entries { // entry has finished, retrieving the last property of the entry
			if j == len(*point) {
				(*point)[j - 1], err = strconv.ParseFloat(s, 64)
				if err != nil {
					WarningLoggerPtr.Println("overflow!")
					err = nil
				}
				full_s += s + string(separate_entries)
				break
			} else { ErrorLoggerPtr.Fatal("Parsing failed") }
		} else {
			s += string(char)
		}
	}
	return full_s, err
}
// Searching the first entry
func Get_actual_begin(load_ptr *[]byte, separate_entries byte) (int64, error) {
	reader := bytes.NewReader(*load_ptr)
	buffered_read := bufio.NewReader(reader)
	found := false
	var i int64 = 0
	// TODO, see the next function
	for char, err := buffered_read.ReadByte(); err == nil && found == false; char, err = buffered_read.ReadByte() {
		// Just searching the character
		if char == separate_entries { found = true }
		i += 1
	}
	if found == true {
		return i, nil
	} else { return i, errors.New("Separate entries not found") }
}
// Searching the last entry, that is the first entry after the offset (length of the slice)
func Get_actual_end(load_ptr *[]byte, separate_entries byte, offset int64) (int64, error) {
	reader := bytes.NewReader((*load_ptr)[offset - 1:])
	buffered_read := bufio.NewReader(reader)
	found := false
	var i int64 = -1
	for char, err := buffered_read.ReadByte(); err == nil; char, err = buffered_read.ReadByte() { // TODO check the error if != EOF
		if char == separate_entries {
			found = true
			break
		}
		i += 1
	}
	if found == true {
		return offset + i, nil
	} else { return offset + i, errors.New("Separate entries not found") }
}

// Online mean
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
// This version of go doesn't support generics
func MinOf_int32(vars ...int32) int32 {
    min := vars[0]

    for _, i := range vars {
        if min > i {
            min = i
        }
    }

    return min
}
// Following function's auxiliary function to check compatibility between parameters
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

// Function for calling a function by its name
func Call(funcName string, stub_storage StubMapping, params ... interface{}) (result interface{}, err error) {
	f := reflect.ValueOf(stub_storage[funcName])
	funcType := reflect.TypeOf(stub_storage[funcName])
	if len(params) != f.Type().NumIn() {
		err = fmt.Errorf("The number of params is out of index, params: %v, needed: %v.",  len(params), f.Type().NumIn())
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
		// if a parameter is nil then reflect can't understand its kind so we must manually create the expected element
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
// Download a slice of an http resource
 func Http_download(resource string, begin int64, end int64) (*[]byte) {
	 req, _ := http.NewRequest("GET", resource, nil)
	 req.Header.Add("Range", "bytes=" +  strconv.FormatInt(begin, 10) + "-" + strconv.FormatInt(end, 10))
	 //fmt.Println(req)
	 var client http.Client
	 resp, err := client.Do(req)
	 if err != nil { ErrorLoggerPtr.Fatal(err) }
	 //fmt.Println(resp)
	 body, err1 := ioutil.ReadAll(resp.Body)
	 if err1 != nil { ErrorLoggerPtr.Fatal(err1) }
	 //fmt.Println(len(body))
	 return &body
 }
