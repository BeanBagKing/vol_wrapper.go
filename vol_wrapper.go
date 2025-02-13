package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	runningModules sync.Map
)

func runModule(volatilityPath, memoryImage, module, outputDir string, wg *sync.WaitGroup) {
	defer wg.Done()

	start := time.Now()
	runningModules.Store(module, start)
	defer runningModules.Delete(module)

	memoryImageName := memoryImage[strings.LastIndex(memoryImage, string(os.PathSeparator))+1:]
	outputFile := fmt.Sprintf("%s%c%s_%s.csv", outputDir, os.PathSeparator, memoryImageName, module)

	cmd := exec.Command(volatilityPath, "-f", memoryImage, "-r", "csv", module)
	outfile, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Error creating output file for module %s: %v\n", module, err)
		return
	}
	defer outfile.Close()

	cmd.Stdout = outfile
	cmd.Stderr = nil // Suppress progress output

	fmt.Printf("Running module: %s\n", module)
	if err := cmd.Run(); err != nil {
		fmt.Printf("!--- Error running module %s: %v\n", module, err)
	} else {
		duration := time.Since(start).Seconds()
		fmt.Printf("    Module %s completed in %.2f seconds\n", module, duration)
	}
}

func monitorKeyPress() {
	reader := bufio.NewReader(os.Stdin)
	for {
		_, err := reader.ReadByte() // Wait for a key press
		if err == nil {
			fmt.Println("\n->->->->->->->->->-> Currently running modules <-<-<-<-<-<-<-<-<-<-")
			runningModules.Range(func(key, value interface{}) bool {
				module := key.(string)
				start := value.(time.Time)
				runtime := time.Since(start).Seconds()
				fmt.Printf("Module: %s, Runtime: %.2f seconds\n", module, runtime)
				return true
			})
			fmt.Println("->->->->->->->->->->->->->->-> End <-<-<-<-<-<-<-<-<-<-<-<-<-<-<-\n")
		}
	}
}

func main() {
	// Define flags
	volatilityPath := flag.String("p", "", "Path to the Volatility3 executable")
	memoryImage := flag.String("i", "", "Path to the memory image")
	modulesFile := flag.String("m", "", "Path to file containing list of modules (newline delimited)")
	outputDir := flag.String("o", "", "Path to the output directory")


	// Override the default usage function
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Custom Help for Volatility Wrapper

		Usage:
		  %s [options]

		Options:
		`, os.Args[0])
				flag.PrintDefaults()
				fmt.Fprintln(os.Stderr, `
		Additional Information:
		  - Enter/Return/↵ during execution will print currently running modules
		  - Example usage:
		      $ go run vol_wrapper.go -p /path/to/which/vol -i /path/to/image.dd -m /path/to/modules.txt -o /path/to/output/folder/
		  - Developed under Linux, may or may not work in Windows
		`)
			}

	flag.Parse()

	if *volatilityPath == "" || *memoryImage == "" || *modulesFile == "" || *outputDir == "" {
		fmt.Println("All flags (-p, -i, -m, -o) are required.")
		os.Exit(1)
	}

	// Ensure the output directory exists
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Read modules from the file
	file, err := os.Open(*modulesFile)
	if err != nil {
		fmt.Printf("Error reading modules file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	modules := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		module := scanner.Text()
		if module != "" {
			modules = append(modules, module)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error scanning modules file: %v\n", err)
		os.Exit(1)
	}

	// Get the number of logical processors and limit the number of goroutines
	numGoroutines := runtime.NumCPU() - 1
	if numGoroutines < 1 {
		numGoroutines = 1
	}

	fmt.Printf("Using up to %d goroutines\n", numGoroutines)

	// Track total time
	totalStart := time.Now()

	// Start key press monitoring in a separate goroutine
	go monitorKeyPress()

	// Create a channel to limit concurrency
	sem := make(chan struct{}, numGoroutines)
	var wg sync.WaitGroup

	// Run each module in a goroutine
	for _, module := range modules {
		sem <- struct{}{} // Acquire a spot in the semaphore
		wg.Add(1)
		go func(module string) {
			runModule(*volatilityPath, *memoryImage, module, *outputDir, &wg)
			<-sem // Release the spot in the semaphore
		}(module)
	}

	wg.Wait() // Wait for all goroutines to complete
	totalDuration := time.Since(totalStart).Seconds()
	fmt.Printf("All modules completed in %.2f seconds.\n", totalDuration)
}

