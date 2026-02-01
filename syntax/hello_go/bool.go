package main

import "fmt"

func test(a bool, b bool) {
	//var a = false
	//var b = false
	var c = !(a && b)
	var d = !(a || b)

	var e = !a || !b
	println(c, d, e)
	println(c == d == e)

	fmt.Printf("🚀 ~ file: bool.go ~ line 17 ~ a: %#v\n", a)
	fmt.Printf("🚀 ~ file: bool.go ~ line 20 ~ a: %+v\n", a)

}
