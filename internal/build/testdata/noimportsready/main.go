// Command noimportsready is a build fixture for gosd-42vb's ImportsPackage
// test: an ordinary main package with no dependency on
// github.com/jphastings/gosd/ready at all.
package main

func main() {
	println("hello from noimportsready")
}
