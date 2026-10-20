package main

import (
	"fmt"
	"os"

	"mal-ledger/pkg/ledger"
)

func main() {
	fmt.Println("================================================================================")
	fmt.Println("             MAL.AI IN-MEMORY ACCOUNT LEDGER CORE - 6-DAY REPLAY                ")
	fmt.Println("================================================================================")

	cfg := ledger.DefaultConfig()
	runner := ledger.NewReplayRunner(cfg)
	printer := ledger.NewReportPrinter(os.Stdout)

	for day := 1; day <= 6; day++ {
		runner.ReplayDay(day)
		printer.PrintDaySummary(runner.Engine, day)
	}

	fmt.Println("\nSimulation completed successfully.")
	fmt.Println("Total append-only entries posted:", len(runner.Engine.Entries))
	fmt.Println("Total fees assessed:            ", len(runner.Engine.FeeAssessments))
	fmt.Println("Total rejections/errors logged:  ", len(runner.Engine.Errors))
}
