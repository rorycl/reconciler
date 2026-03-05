// reconciler-dev is a version of the reconciler app for local development.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rorycl/reconciler/app"
)

// appInitialiser converts an app.NewApp to a cli WebRunner interface.
func appInitialiser(
	configFile string,
	logLevel slog.Level,
	inDevelopment bool,
	staticPath, templatePath, sqlPath, databasePath string,
) (WebRunner, error) {
	return app.NewApp(configFile, logLevel, inDevelopment, staticPath, templatePath, sqlPath, databasePath)
}

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
		fmt.Printf("To proceed please input the code %s at the prompt.\n", code)
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
		}
	}()

	select {
	case <-time.After(10 * time.Second):
		return errors.New("no code was input in 10 seconds. aborting")
	case <-aChan:
		break
	}
	return nil
}

func run(args []string) error {

	// BuildCLI builds the command line application, injecting the app constructor for
	// filling with cli arguments.
	cmd := BuildCLI(AppMaker(appInitialiser), getPin)

	ctx := context.Background()

	// Run runs the production webserver.
	return cmd.Run(ctx, args)
}

func main() {
	err := run(os.Args)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
