// Command gosd-init is PID 1 on a gosd image: it brings up the board and
// execs the user's application. This is a placeholder; the real init
// sequence (mounting partitions, network bring-up, exec'ing the app) is
// implemented by a later bean.
package main

func main() {
	println("gosd-init: not yet implemented")
}
