package main

func Assert(condition bool, msg string) {
	if !condition {
		panic(msg)
	}
}

func AssertNotReachable(msg string) {
	panic(msg)
}

func Require(condition bool, msg string) {
	if !condition {
		panic(msg)
	}
}
