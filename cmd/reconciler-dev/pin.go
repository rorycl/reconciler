package main

import (
	"bufio"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

var pinTimeout = 10 * time.Second

// getPin is a security checker to help ensure that the development binary is not used
// in production.
func getPin() error {

	sequence := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}

	code := ""
	for range 6 {
		r := rand.Intn(len(sequence))
		code += strconv.Itoa(sequence[r])
	}

	aChan := make(chan struct{})

	go func() {
		fmt.Printf("To proceed please input the code at the prompt.\n%s\n", code)
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("> ")
			text, _ := reader.ReadString('\n')
			if strings.Contains(text, code) {
				fmt.Printf("%s\n\n", "champ")
				close(aChan)
				return
			} else {
				fmt.Println("nil points\n>")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-time.After(pinTimeout):
		return errors.New("no code was input in 10 seconds. aborting")
	case <-aChan:
		break
	}
	return nil
}
