package echo

import (
	"fmt"
	"os"
	"testing"
)

func writeCoverageReport(coverage map[int]bool, total int, functionName string) {
	file, err := os.Create("coverage/coverage_" + functionName + ".txt")
	if err != nil {
		fmt.Println("Failed to create coverage report file:", err)
		return
	}
	defer file.Close()

	fmt.Println("\n--- Branch Coverage Report ---")
	file.WriteString("--- Branch Coverage Report ---\n")

	executedBranches := 0
	for id, executed := range coverage {
		if executed {
			executedBranches++
		}
		line := fmt.Sprintf("Branch %d executed: %v\n", id, executed)
		fmt.Print(line)
		file.WriteString(line)
	}

	percentage := (float64(executedBranches) / float64(total)) * 100

	fmt.Println("Branch coverage:", percentage, "%")
	file.WriteString(fmt.Sprintf("Branch coverage: %.2f%%\n", percentage))

	fmt.Println("--- End of Report ---")
	file.WriteString("--- End of Report ---\n")
}

func initCoverage(coverage map[int]bool, total int) {
	for i := 0; i < total; i++ {
		coverage[i] = false
	}
}

func TestMain(m *testing.M) {
	initCoverage(bindDataCoverage, bindDataCoverageTotal)
	initCoverage(insertNodeCoverage, insertNodeCoverageTotal)

	exitCode := m.Run()

	writeCoverageReport(bindDataCoverage, bindDataCoverageTotal, "bindData")
	writeCoverageReport(insertNodeCoverage, insertNodeCoverageTotal, "insertNode")

	os.Exit(exitCode)
}
