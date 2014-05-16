package errgo_test

import (
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"
	"testing"

	"github.com/juju/errgo"
)

var (
	_ errgo.Wrapper    = (*errgo.Err)(nil)
	_ errgo.Locationer = (*errgo.Err)(nil)
	_ errgo.Causer     = (*errgo.Err)(nil)
)

func TestNew(t *testing.T) {
	err := errgo.New("foo") //err TestNew
	checkErr(t, err, nil, "foo", "[{$TestNew$: foo}]", err)
}

func TestNewf(t *testing.T) {
	err := errgo.Newf("foo %d", 5) //err TestNewf
	checkErr(t, err, nil, "foo 5", "[{$TestNewf$: foo 5}]", err)
}

var someErr = errgo.New("some error") //err varSomeErr

func annotate1() error {
	err := errgo.Notef(someErr, "annotate1") //err annotate1
	return err
}

func annotate2() error {
	err := annotate1()
	err = errgo.Notef(err, "annotate2") //err annotate2
	return err
}

func TestNoteUsage(t *testing.T) {
	err0 := annotate2()
	err, ok := err0.(errgo.Wrapper)
	if !ok {
		t.Fatalf("expected an errgo.Wrapper got %#v", err0)
	}
	underlying := err.Underlying()
	checkErr(
		t, err0, underlying,
		"annotate2: annotate1: some error",
		"[{$annotate2$: annotate2} {$annotate1$: annotate1} {$varSomeErr$: some error}]",
		err0)
}

func TestMask(t *testing.T) {
	err0 := errgo.WithCausef(nil, someErr, "foo") //err TestMask#0
	err := errgo.Mask(err0)                       //err TestMask#1
	checkErr(t, err, err0, "foo", "[{$TestMask#1$: } {$TestMask#0$: foo}]", err)

	err = errgo.Mask(nil)
	if err != nil {
		t.Fatalf("expected nil got %#v", err)
	}
}

func TestNotef(t *testing.T) {
	err0 := errgo.WithCausef(nil, someErr, "foo") //err TestNotef#0
	err := errgo.Notef(err0, "bar")               //err TestNotef#1
	checkErr(t, err, err0, "bar: foo", "[{$TestNotef#1$: bar} {$TestNotef#0$: foo}]", err)

	err = errgo.Notef(nil, "bar") //err TestNotef#2
	checkErr(t, err, nil, "bar", "[{$TestNotef#2$: bar}]", err)
}

func TestMaskFunc(t *testing.T) {
	err0 := errgo.New("zero")
	err1 := errgo.New("one")

	allowVals := func(vals ...error) (r []func(error) bool) {
		for _, val := range vals {
			r = append(r, errgo.Is(val))
		}
		return
	}
	tests := []struct {
		err    error
		allow0 []func(error) bool
		allow1 []func(error) bool
		cause  error
	}{{
		err:    err0,
		allow0: allowVals(err0),
		cause:  err0,
	}, {
		err:    err1,
		allow0: allowVals(err0),
		cause:  nil,
	}, {
		err:    err0,
		allow1: allowVals(err0),
		cause:  err0,
	}, {
		err:    err0,
		allow0: allowVals(err1),
		allow1: allowVals(err0),
		cause:  err0,
	}, {
		err:    err0,
		allow0: allowVals(err0, err1),
		cause:  err0,
	}, {
		err:    err1,
		allow0: allowVals(err0, err1),
		cause:  err1,
	}, {
		err:    err0,
		allow1: allowVals(err0, err1),
		cause:  err0,
	}, {
		err:    err1,
		allow1: allowVals(err0, err1),
		cause:  err1,
	}}
	for i, test := range tests {
		wrap := errgo.MaskFunc(test.allow0...)
		err := wrap(test.err, test.allow1...)
		cause := errgo.Cause(err)
		wantCause := test.cause
		if wantCause == nil {
			wantCause = err
		}
		if cause != wantCause {
			t.Errorf("test %d. got %#v want %#v", i, cause, err)
		}
	}
}

type embed struct {
	*errgo.Err
}

func TestCause(t *testing.T) {
	if cause := errgo.Cause(someErr); cause != someErr {
		t.Fatalf("expected %q kind; got %#v", someErr, cause)
	}
	causeErr := errgo.New("cause error")
	underlyingErr := errgo.New("underlying error")                 //err TestCause#1
	err := errgo.WithCausef(underlyingErr, causeErr, "foo %d", 99) //err TestCause#2
	if errgo.Cause(err) != causeErr {
		t.Fatalf("expected %q; got %#v", causeErr, errgo.Cause(err))
	}
	checkErr(t, err, underlyingErr, "foo 99: underlying error", "[{$TestCause#2$: foo 99} {$TestCause#1$: underlying error}]", causeErr)
	err = &embed{err.(*errgo.Err)}
	if errgo.Cause(err) != causeErr {
		t.Fatalf("expected %q; got %#v", causeErr, errgo.Cause(err))
	}
}

func TestDetails(t *testing.T) {
	if details := errgo.Details(nil); details != "[]" {
		t.Fatalf("errgo.Details(nil) got %q want %q", details, "[]")
	}

	otherErr := fmt.Errorf("other")
	checkErr(t, otherErr, nil, "other", "[{other}]", otherErr)

	err0 := &embed{errgo.New("foo").(*errgo.Err)} //err TestStack#0
	checkErr(t, err0, nil, "foo", "[{$TestStack#0$: foo}]", err0)

	err1 := &embed{errgo.Notef(err0, "bar").(*errgo.Err)} //err TestStack#1
	checkErr(t, err1, err0, "bar: foo", "[{$TestStack#1$: bar} {$TestStack#0$: foo}]", err1)

	err2 := errgo.Mask(err1) //err TestStack#2
	checkErr(t, err2, err1, "bar: foo", "[{$TestStack#2$: } {$TestStack#1$: bar} {$TestStack#0$: foo}]", err2)
}

func TestMatch(t *testing.T) {
	type errTest func(error) bool
	allow := func(ss ...string) []func(error) bool {
		fns := make([]func(error) bool, len(ss))
		for i, s := range ss {
			s := s
			fns[i] = func(err error) bool {
				return err != nil && err.Error() == s
			}
		}
		return fns
	}
	tests := []struct {
		err error
		fns []func(error) bool
		ok  bool
	}{{
		err: errgo.New("foo"),
		fns: allow("foo"),
		ok:  true,
	}, {
		err: errgo.New("foo"),
		fns: allow("bar"),
		ok:  false,
	}, {
		err: errgo.New("foo"),
		fns: allow("bar", "foo"),
		ok:  true,
	}, {
		err: errgo.New("foo"),
		fns: nil,
		ok:  false,
	}, {
		err: nil,
		fns: nil,
		ok:  false,
	}}

	for i, test := range tests {
		ok := errgo.Match(test.err, test.fns...)
		if ok != test.ok {
			t.Fatalf("test %d: expected %v got %v", i, test.ok, ok)
		}
	}
}

func TestLocation(t *testing.T) {
	loc := errgo.Location{"foo", 35}
	if loc.String() != "foo:35" {
		t.Fatalf("expected \"foo:35\" got %q", loc.String)
	}
}

func checkErr(t *testing.T, err, underlying error, msg string, details string, cause error) {
	if err == nil {
		t.Fatalf("err is nil; want %q", msg)
	}
	if err.Error() != msg {
		t.Fatalf("unexpected message: want %q; got %q", msg, err.Error())
	}
	if err, ok := err.(errgo.Wrapper); ok {
		if err.Underlying() != underlying {
			t.Fatalf("unexpected underlying error: want %q; got %v", underlying, err.Underlying())
		}
	} else if underlying != nil {
		t.Fatalf("no underlying error found; want %q", underlying)
	}
	if errgo.Cause(err) != cause {
		t.Fatalf("unexpected cause: want %#v; got %#v", cause, errgo.Cause(err))
	}
	wantDetails := replaceLocations(details)
	if gotDetails := errgo.Details(err); gotDetails != wantDetails {
		t.Fatalf("unexpected details: want %q; got %q", wantDetails, gotDetails)
	}
}

func replaceLocations(s string) string {
	t := ""
	for {
		i := strings.Index(s, "$")
		if i == -1 {
			break
		}
		t += s[0:i]
		s = s[i+1:]
		i = strings.Index(s, "$")
		if i == -1 {
			panic("no second $")
		}
		t += location(s[0:i]).String()
		s = s[i+1:]
	}
	t += s
	return t
}

func location(tag string) errgo.Location {
	line, ok := tagToLine[tag]
	if !ok {
		panic(fmt.Errorf("tag %q not found", tag))
	}
	return errgo.Location{
		File: filename,
		Line: line,
	}
}

var tagToLine = make(map[string]int)
var filename string

func init() {
	data, err := ioutil.ReadFile("errors_test.go")
	if err != nil {
		panic(err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if j := strings.Index(line, "//err "); j >= 0 {
			tagToLine[line[j+len("//err "):]] = i + 1
		}
	}
	_, filename, _, _ = runtime.Caller(0)
}
