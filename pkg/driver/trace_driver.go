/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package driver

import (
	"container/list"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/driver/clients"
	"github.com/vhive-serverless/loader/pkg/driver/deployment"
	"github.com/vhive-serverless/loader/pkg/driver/failure"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/generator"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"github.com/vhive-serverless/loader/pkg/trace"
)

type Driver struct {
	Configuration          *config.Configuration
	SpecificationGenerator *generator.SpecificationGenerator
	Invoker                clients.Invoker

	readOpenWhiskMetadata sync.Mutex
	allFunctionsInvoked   sync.WaitGroup
}

func NewDriver(driverConfig *config.Configuration) *Driver {
	d := &Driver{
		Configuration:          driverConfig,
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.LoaderConfiguration.Seed),

		readOpenWhiskMetadata: sync.Mutex{},
		allFunctionsInvoked:   sync.WaitGroup{},
	}

	d.Invoker = clients.CreateInvoker(driverConfig.LoaderConfiguration, &d.allFunctionsInvoked, &d.readOpenWhiskMetadata)

	return d
}

// ///////////////////////////////////////
// HELPER METHODS
// ///////////////////////////////////////
func (d *Driver) outputFilename(name string) string {
	return fmt.Sprintf("%s_%s_%d.csv", d.Configuration.LoaderConfiguration.OutputPathPrefix, name, d.Configuration.TraceDuration)
}

func (d *Driver) runCSVWriter(records chan interface{}, filename string, writerDone *sync.WaitGroup) {
	log.Debugf("Starting writer for %s", filename)

	file, err := os.Create(filename)
	common.Check(err)
	defer file.Close()

	writer := gocsv.NewSafeCSVWriter(csv.NewWriter(file))
	if err := gocsv.MarshalChan(records, writer); err != nil {
		log.Fatal(err)
	}

	writerDone.Done()
}

func DAGCreation(functions []*common.Function) *list.List {
	linkedList := list.New()
	// Assigning nodes one after another
	for _, function := range functions {
		linkedList.PushBack(function)
	}
	return linkedList
}

/////////////////////////////////////////
// METRICS SCRAPPERS
/////////////////////////////////////////

func (d *Driver) CreateMetricsScrapper(interval time.Duration,
	signalReady *sync.WaitGroup, finishCh chan int, allRecordsWritten *sync.WaitGroup) func() {
	timer := time.NewTicker(interval)

	return func() {
		signalReady.Done()
		knStatRecords := make(chan interface{}, 100)
		scaleRecords := make(chan interface{}, 100)
		writerDone := sync.WaitGroup{}

		clusterUsageFile, err := os.Create(d.outputFilename("cluster_usage"))
		common.Check(err)
		defer clusterUsageFile.Close()

		writerDone.Add(1)
		go d.runCSVWriter(knStatRecords, d.outputFilename("kn_stats"), &writerDone)

		writerDone.Add(1)
		go d.runCSVWriter(scaleRecords, d.outputFilename("deployment_scale"), &writerDone)

		for {
			select {
			case <-timer.C:
				recCluster := mc.ScrapeClusterUsage()
				recCluster.Timestamp = time.Now().UnixMicro()

				byteArr, err := json.Marshal(recCluster)
				common.Check(err)

				_, err = clusterUsageFile.Write(byteArr)
				common.Check(err)

				_, err = clusterUsageFile.WriteString("\n")
				common.Check(err)

				recScale := mc.ScrapeDeploymentScales()
				timestamp := time.Now().UnixMicro()
				for _, rec := range recScale {
					rec.Timestamp = timestamp
					scaleRecords <- rec
				}

				recKnative := mc.ScrapeKnStats()
				recKnative.Timestamp = time.Now().UnixMicro()
				knStatRecords <- recKnative
			case <-finishCh:
				close(knStatRecords)
				close(scaleRecords)

				writerDone.Wait()
				allRecordsWritten.Done()

				return
			}
		}
	}
}

/////////////////////////////////////////
// DRIVER LOGIC
/////////////////////////////////////////

type InvocationMetadata struct {
	RootFunction *list.List
	Phase        common.ExperimentPhase

	MinuteIndex     int
	InvocationIndex int

	SuccessCount        *int64
	FailedCount         *int64
	FailedCountByMinute []int64

	RecordOutputChannel   chan interface{}
	AnnounceDoneWG        *sync.WaitGroup
	AnnounceDoneExe       *sync.WaitGroup
	ReadOpenWhiskMetadata *sync.Mutex
}

func composeInvocationID(timeGranularity common.TraceGranularity, minuteIndex int, invocationIndex int) string {
	var timePrefix string

	switch timeGranularity {
	case common.MinuteGranularity:
		timePrefix = "min"
	case common.SecondGranularity:
		timePrefix = "sec"
	default:
		log.Fatal("Invalid trace granularity parameter.")
	}

	return fmt.Sprintf("%s%d.inv%d", timePrefix, minuteIndex, invocationIndex)
}

func (d *Driver) invokeFunction(metadata *InvocationMetadata) {
	defer metadata.AnnounceDoneWG.Done()

	var success bool
	node := metadata.RootFunction.Front()
	var record *mc.ExecutionRecord
	var runtimeSpecifications *common.RuntimeSpecification
	for node != nil {
		function := node.Value.(*common.Function)
		runtimeSpecifications = &function.Specification.RuntimeSpecification[metadata.MinuteIndex][metadata.InvocationIndex]

		success, record = d.Invoker.Invoke(function, runtimeSpecifications)

		record.Phase = int(metadata.Phase)
		record.InvocationID = composeInvocationID(d.Configuration.TraceGranularity, metadata.MinuteIndex, metadata.InvocationIndex)
		metadata.RecordOutputChannel <- record

		if !success {
			log.Debugf("Invocation failed at minute: %d for %s", metadata.MinuteIndex, function.Name)
			break
		}
		node = node.Next()
	}
	if success {
		atomic.AddInt64(metadata.SuccessCount, 1)
	} else {
		atomic.AddInt64(metadata.FailedCount, 1)
		atomic.AddInt64(&metadata.FailedCountByMinute[metadata.MinuteIndex], 1)
	}
}

func (d *Driver) functionsDriver(list *list.List, announceFunctionDone *sync.WaitGroup,
	addInvocationsToGroup *sync.WaitGroup, readOpenWhiskMetadata *sync.Mutex, totalSuccessful *int64,
	totalFailed *int64, totalIssued *int64, recordOutputChannel chan interface{}) {

	function := list.Front().Value.(*common.Function)
	numberOfInvocations := 0
	for i := 0; i < len(function.InvocationStats.Invocations); i++ {
		numberOfInvocations += function.InvocationStats.Invocations[i]
	}
	addInvocationsToGroup.Add(numberOfInvocations)

	totalTraceDuration := d.Configuration.TraceDuration
	minuteIndex, invocationIndex := 0, 0

	IAT := function.Specification.IAT

	var successfulInvocations int64
	var failedInvocations int64
	var failedInvocationByMinute = make([]int64, totalTraceDuration)
	var numberOfIssuedInvocations int64
	var currentPhase = common.ExecutionPhase

	waitForInvocations := sync.WaitGroup{}

	if d.Configuration.WithWarmup() {
		currentPhase = common.WarmupPhase
		// skip the first minute because of profiling
		minuteIndex = 1

		log.Infof("Warmup phase has started.")
	}

	startOfMinute := time.Now()
	var previousIATSum int64

	for {
		if minuteIndex >= totalTraceDuration {
			// Check whether the end of trace has been reached
			break
		} else if function.InvocationStats.Invocations[minuteIndex] == 0 {
			// Sleep for a minute if there are no invocations
			if d.proceedToNextMinute(function, &minuteIndex, &invocationIndex,
				&startOfMinute, true, &currentPhase, failedInvocationByMinute, &previousIATSum) {
				break
			}

			switch d.Configuration.TraceGranularity {
			case common.MinuteGranularity:
				time.Sleep(time.Minute)
			case common.SecondGranularity:
				time.Sleep(time.Second)
			default:
				log.Fatal("Unsupported trace granularity.")
			}

			continue
		}

		iat := time.Duration(IAT[minuteIndex][invocationIndex]) * time.Microsecond

		currentTime := time.Now()
		schedulingDelay := currentTime.Sub(startOfMinute).Microseconds() - previousIATSum
		sleepFor := iat.Microseconds() - schedulingDelay
		time.Sleep(time.Duration(sleepFor) * time.Microsecond)

		previousIATSum += iat.Microseconds()

		if function.InvocationStats.Invocations[minuteIndex] == invocationIndex || hasMinuteExpired(startOfMinute) {
			readyToBreak := d.proceedToNextMinute(function, &minuteIndex, &invocationIndex, &startOfMinute,
				false, &currentPhase, failedInvocationByMinute, &previousIATSum)

			if readyToBreak {
				break
			}
		} else {
			if !d.Configuration.TestMode {
				waitForInvocations.Add(1)

				go d.invokeFunction(&InvocationMetadata{
					RootFunction:          list,
					Phase:                 currentPhase,
					MinuteIndex:           minuteIndex,
					InvocationIndex:       invocationIndex,
					SuccessCount:          &successfulInvocations,
					FailedCount:           &failedInvocations,
					FailedCountByMinute:   failedInvocationByMinute,
					RecordOutputChannel:   recordOutputChannel,
					AnnounceDoneWG:        &waitForInvocations,
					AnnounceDoneExe:       addInvocationsToGroup,
					ReadOpenWhiskMetadata: readOpenWhiskMetadata,
				})
			} else {
				// To be used from within the Golang testing framework
				log.Debugf("Test mode invocation fired.\n")

				recordOutputChannel <- &mc.ExecutionRecordBase{
					Phase:        int(currentPhase),
					InvocationID: composeInvocationID(d.Configuration.TraceGranularity, minuteIndex, invocationIndex),
					StartTime:    time.Now().UnixNano(),
				}

				successfulInvocations++
			}
			numberOfIssuedInvocations++
			invocationIndex++
		}
	}

	waitForInvocations.Wait()

	log.Debugf("All the invocations for function %s have been completed.\n", function.Name)
	announceFunctionDone.Done()

	atomic.AddInt64(totalSuccessful, successfulInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
	atomic.AddInt64(totalIssued, numberOfIssuedInvocations)
}

func (d *Driver) proceedToNextMinute(function *common.Function, minuteIndex *int, invocationIndex *int, startOfMinute *time.Time,
	skipMinute bool, currentPhase *common.ExperimentPhase, failedInvocationByMinute []int64, previousIATSum *int64) bool {

	if d.Configuration.TraceGranularity == common.MinuteGranularity {
		if !isRequestTargetAchieved(function.InvocationStats.Invocations[*minuteIndex], *invocationIndex, common.RequestedVsIssued) {
			// Not fatal because we want to keep the measurements to be written to the output file
			log.Warnf("Relative difference between requested and issued number of invocations is greater than %.2f%%. Terminating function driver for %s!\n", common.RequestedVsIssuedTerminateThreshold*100, function.Name)

			return true
		}

		for i := 0; i <= *minuteIndex; i++ {
			notFailedCount := function.InvocationStats.Invocations[i] - int(atomic.LoadInt64(&failedInvocationByMinute[i]))
			if !isRequestTargetAchieved(function.InvocationStats.Invocations[i], notFailedCount, common.IssuedVsFailed) {
				// Not fatal because we want to keep the measurements to be written to the output file
				log.Warnf("Percentage of failed request is greater than %.2f%%. Terminating function driver for %s!\n", common.FailedTerminateThreshold*100, function.Name)

				return true
			}
		}
	}

	*minuteIndex++
	*invocationIndex = 0
	*previousIATSum = 0

	if d.Configuration.WithWarmup() && *minuteIndex == (d.Configuration.LoaderConfiguration.WarmupDuration+1) {
		*currentPhase = common.ExecutionPhase
		log.Infof("Warmup phase has finished. Starting the execution phase.")
	}

	if !skipMinute {
		*startOfMinute = time.Now()
	} else {
		switch d.Configuration.TraceGranularity {
		case common.MinuteGranularity:
			*startOfMinute = time.Now().Add(time.Minute)
		case common.SecondGranularity:
			*startOfMinute = time.Now().Add(time.Second)
		default:
			log.Fatal("Unsupported trace granularity.")
		}
	}

	return false
}

func isRequestTargetAchieved(ideal int, real int, assertType common.RuntimeAssertType) bool {
	if ideal == 0 {
		return true
	}

	ratio := float64(ideal-real) / float64(ideal)

	var warnBound float64
	var terminationBound float64
	var warnMessage string

	switch assertType {
	case common.RequestedVsIssued:
		warnBound = common.RequestedVsIssuedWarnThreshold
		terminationBound = common.RequestedVsIssuedTerminateThreshold
		warnMessage = fmt.Sprintf("Relative difference between requested and issued number of invocations has reached %.2f.", ratio)
	case common.IssuedVsFailed:
		warnBound = common.FailedWarnThreshold
		terminationBound = common.FailedTerminateThreshold
		warnMessage = fmt.Sprintf("Percentage of failed invocations within a minute has reached %.2f.", ratio)
	default:
		log.Fatal("Invalid type of assertion at runtime.")
	}

	if ratio < 0 || ratio > 1 {
		log.Fatalf("Invalid arguments provided to runtime assertion.\n")
	} else if ratio >= terminationBound {
		return false
	}

	if ratio >= warnBound && ratio < terminationBound {
		log.Warn(warnMessage)
	}

	return true
}

func hasMinuteExpired(t1 time.Time) bool {
	return time.Since(t1) > time.Minute
}

func (d *Driver) globalTimekeeper(totalTraceDuration int, signalReady *sync.WaitGroup) {
	ticker := time.NewTicker(time.Minute)
	globalTimeCounter := 0

	signalReady.Done()

	for {
		<-ticker.C

		log.Debugf("End of minute %d\n", globalTimeCounter)
		globalTimeCounter++
		if globalTimeCounter >= totalTraceDuration {
			break
		}

		log.Debugf("Start of minute %d\n", globalTimeCounter)
	}

	ticker.Stop()
}

func (d *Driver) createGlobalMetricsCollector(filename string, collector chan interface{},
	signalReady *sync.WaitGroup, signalEverythingWritten *sync.WaitGroup, totalIssuedChannel chan int64) {

	// NOTE: totalNumberOfInvocations is initialized to MaxInt64 not to allow collector to complete before
	// the end signal is received on totalIssuedChannel, which deliver the total number of issued invocations.
	// This number is known once all the individual function drivers finish issuing invocations and
	// when all the invocations return
	var totalNumberOfInvocations int64 = math.MaxInt64
	var currentlyWritten int64

	file, err := os.Create(filename)
	common.Check(err)
	defer file.Close()
	signalReady.Done()

	records := make(chan interface{}, 100)
	writerDone := sync.WaitGroup{}
	writerDone.Add(1)
	go d.runCSVWriter(records, filename, &writerDone)

	for {
		select {
		case record := <-collector:
			records <- record

			currentlyWritten++
		case record := <-totalIssuedChannel:
			totalNumberOfInvocations = record
		}

		if currentlyWritten == totalNumberOfInvocations {
			close(records)
			writerDone.Wait()
			(*signalEverythingWritten).Done()

			return
		}
	}
}

func (d *Driver) startBackgroundProcesses(allRecordsWritten *sync.WaitGroup) (*sync.WaitGroup, chan interface{}, chan int64, chan int) {
	auxiliaryProcessBarrier := &sync.WaitGroup{}

	finishCh := make(chan int, 1)

	if d.Configuration.LoaderConfiguration.EnableMetricsScrapping {
		auxiliaryProcessBarrier.Add(1)

		allRecordsWritten.Add(1)
		metricsScrapper := d.CreateMetricsScrapper(time.Second*time.Duration(d.Configuration.LoaderConfiguration.MetricScrapingPeriodSeconds), auxiliaryProcessBarrier, finishCh, allRecordsWritten)
		go metricsScrapper()
	}

	auxiliaryProcessBarrier.Add(2)

	globalMetricsCollector := make(chan interface{})
	totalIssuedChannel := make(chan int64)
	go d.createGlobalMetricsCollector(d.outputFilename("duration"), globalMetricsCollector, auxiliaryProcessBarrier, allRecordsWritten, totalIssuedChannel)

	traceDurationInMinutes := d.Configuration.TraceDuration
	go d.globalTimekeeper(traceDurationInMinutes, auxiliaryProcessBarrier)

	return auxiliaryProcessBarrier, globalMetricsCollector, totalIssuedChannel, finishCh
}

func (d *Driver) internalRun(iatOnly bool, generated bool) {
	var successfulInvocations int64
	var failedInvocations int64
	var invocationsIssued int64
	var functionsPerDAG int64
	readOpenWhiskMetadata := sync.Mutex{}
	allFunctionsInvoked := sync.WaitGroup{}
	allIndividualDriversCompleted := sync.WaitGroup{}
	allRecordsWritten := sync.WaitGroup{}
	allRecordsWritten.Add(1)

	backgroundProcessesInitializationBarrier, globalMetricsCollector, totalIssuedChannel, scraperFinishCh := d.startBackgroundProcesses(&allRecordsWritten)

	if !iatOnly {
		log.Info("Generating IAT and runtime specifications for all the functions")
		for i, function := range d.Configuration.Functions {
			// Equalising all the InvocationStats to the first function
			if d.Configuration.LoaderConfiguration.DAGMode {
				function.InvocationStats.Invocations = d.Configuration.Functions[0].InvocationStats.Invocations
			}
			spec := d.SpecificationGenerator.GenerateInvocationData(
				function,
				d.Configuration.IATDistribution,
				d.Configuration.ShiftIAT,
				d.Configuration.TraceGranularity,
			)

			d.Configuration.Functions[i].Specification = spec
		}
	}

	backgroundProcessesInitializationBarrier.Wait()

	if generated {
		for i := range d.Configuration.Functions {
			var spec common.FunctionSpecification

			iatFile, _ := os.ReadFile("iat" + strconv.Itoa(i) + ".json")
			err := json.Unmarshal(iatFile, &spec)
			if err != nil {
				log.Fatalf("Failed tu unmarshal iat file: %s", err)
			}

			d.Configuration.Functions[i].Specification = &spec
		}
	}

	if d.Configuration.LoaderConfiguration.DAGMode {
		log.Infof("Starting DAG invocation driver\n")
		functionLinkedList := DAGCreation(d.Configuration.Functions)
		functionsPerDAG = int64(len(d.Configuration.Functions))
		allIndividualDriversCompleted.Add(1)
		go d.functionsDriver(
			functionLinkedList,
			&allIndividualDriversCompleted,
			&allFunctionsInvoked,
			&readOpenWhiskMetadata,
			&successfulInvocations,
			&failedInvocations,
			&invocationsIssued,
			globalMetricsCollector,
		)
	} else {
		log.Infof("Starting function invocation driver\n")
		functionsPerDAG = 1
		for _, function := range d.Configuration.Functions {
			allIndividualDriversCompleted.Add(1)
			linkedList := list.New()
			linkedList.PushBack(function)
			go d.functionsDriver(
				linkedList,
				&allIndividualDriversCompleted,
				&allFunctionsInvoked,
				&readOpenWhiskMetadata,
				&successfulInvocations,
				&failedInvocations,
				&invocationsIssued,
				globalMetricsCollector,
			)
		}
	}
	allIndividualDriversCompleted.Wait()
	if atomic.LoadInt64(&successfulInvocations)+atomic.LoadInt64(&failedInvocations) != 0 {
		log.Debugf("Waiting for all the invocations record to be written.\n")

		totalIssuedChannel <- atomic.LoadInt64(&invocationsIssued) * functionsPerDAG
		scraperFinishCh <- 0 // Ask the scraper to finish metrics collection

		allRecordsWritten.Wait()
	}

	log.Infof("Trace has finished executing function invocation driver\n")
	log.Infof("Number of successful invocations: \t%d\n", atomic.LoadInt64(&successfulInvocations))
	log.Infof("Number of failed invocations: \t%d\n", atomic.LoadInt64(&failedInvocations))
}

func (d *Driver) RunExperiment(iatOnly bool, generated bool) {
	if iatOnly {
		log.Info("Generating IAT and runtime specifications for all the functions")
		for i, function := range d.Configuration.Functions {
			spec := d.SpecificationGenerator.GenerateInvocationData(
				function,
				d.Configuration.IATDistribution,
				d.Configuration.ShiftIAT,
				d.Configuration.TraceGranularity,
			)
			d.Configuration.Functions[i].Specification = spec

			file, _ := json.MarshalIndent(spec, "", " ")
			err := os.WriteFile("iat"+strconv.Itoa(i)+".json", file, 0644)
			if err != nil {
				log.Fatalf("Writing the loader config file failed: %s", err)
			}
		}

		log.Info("IATs have been generated. The program has exited.")
		os.Exit(0)
	}

	if d.Configuration.WithWarmup() {
		trace.DoStaticTraceProfiling(d.Configuration.Functions)
	}

	trace.ApplyResourceLimits(d.Configuration.Functions, d.Configuration.LoaderConfiguration.CPULimit)

	deployer := deployment.CreateDeployer(d.Configuration)
	deployer.Deploy(d.Configuration)

	go failure.ScheduleFailure(d.Configuration.LoaderConfiguration.Platform, d.Configuration.FailureConfiguration)

	// Generate load
	d.internalRun(iatOnly, generated)

	// Clean up
	deployer.Clean()
}
