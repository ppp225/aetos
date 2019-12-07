package main

import "github.com/ppp225/aetos"

func main() {
	aetos := aetos.New("aetos.yml")
	// aetos.Debug()
	aetos.Run()
}
