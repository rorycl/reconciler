package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

var pinTimeout = 10 * time.Second

type pin string

func newPin() *pin {
	sequence := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	code := ""
	for range 6 {
		r := rand.Intn(len(sequence))
		code += strconv.Itoa(sequence[r])
	}
	p := pin(code)
	return &p
}

func (p *pin) verify(input string) bool {
	if strings.TrimSpace(input) == string(*p) {
		fmt.Println("champ")
		return true
	}
	fmt.Println("nil points\n>")
	return false
}

var stdin = os.Stdin

func (p *pin) check() error {
	aChan := make(chan struct{})

	go func() {
		fmt.Printf("To proceed please input the code at the prompt.\n%s\n", string(*p))
		reader := bufio.NewReader(stdin)
		for {
			fmt.Print("> ")
			input, _ := reader.ReadString('\n')
			if p.verify(input) {
				close(aChan)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-time.After(pinTimeout):
		return fmt.Errorf("no code was input in %s. aborting", pinTimeout)
	case <-aChan:
		break
	}
	return nil
}
