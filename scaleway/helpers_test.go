package scaleway

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// AssertTrue fails the test if is not true.
func AssertTrue(tb testing.TB, test bool) {
	if !test {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected result: %t (wanted true)\033[39m\n", filepath.Base(file), line, test)
		tb.FailNow()
	}
}

// AssertFalse fails the test if is not false.
func AssertFalse(tb testing.TB, test bool) {
	if test {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected result: %t (wanted false)\033[39m\n", filepath.Base(file), line, test)
		tb.FailNow()
	}
}

// AssertNoError fails the test if an err is not nil.
func AssertNoError(tb testing.TB, err error) {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected error: %s\033[39m\n", filepath.Base(file), line, err.Error())
		tb.FailNow()
	}
}

// Equals fails the test if exp is not equal to act.
func Equals(tb testing.TB, exp, act interface{}) {
	if !reflect.DeepEqual(exp, act) {
		_, file, line, _ := runtime.Caller(1)
		fmt.Printf("\033[31m%s:%d: unexpected result\nexp: %#v\ngot: %#v\033[39m\n", filepath.Base(file), line, exp, act)
		tb.FailNow()
	}
}
