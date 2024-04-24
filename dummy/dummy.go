package dummy

import "fmt"

// All your code goes in this folder

// Run function is required, and takes all commandline args except the name of the command
// like "dummy"
func Run(args []string) {
	fmt.Printf("I'm dummy and my args are: %v", args)
}
