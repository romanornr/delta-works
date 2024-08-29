package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	delta "github.com/romanornr/delta-works/internal/engine/core"
	"github.com/romanornr/delta-works/internal/repository"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/dispatch"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/alert"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/request"
	"github.com/thrasher-corp/gocryptotrader/exchanges/trade"
	"github.com/thrasher-corp/gocryptotrader/gctscript"
	gctscriptVM "github.com/thrasher-corp/gocryptotrader/gctscript/vm"
	gctlog "github.com/thrasher-corp/gocryptotrader/log"
	"github.com/thrasher-corp/gocryptotrader/portfolio/withdraw"
	"github.com/thrasher-corp/gocryptotrader/signaler"
	"log"
	"os"
	"runtime"
	"time"
)

const (
	// DefaultConfigPath is the default configuration file path
	DefaultConfigPath = "config.json"
	botName           = "DeltaWorks"
)

func main() {
	// Handle flags
	var settings engine.Settings
	versionFlag := flag.Bool("version", false, "retrieves current GoCryptoTrader version")

	// Core settings
	flag.StringVar(&settings.ConfigFile, "config", config.DefaultFilePath(), "config file to load")
	flag.StringVar(&settings.DataDir, "datadir", common.GetDefaultDataDir(runtime.GOOS), "default data directory for GoCryptoTrader files")
	flag.IntVar(&settings.GoMaxProcs, "gomaxprocs", runtime.GOMAXPROCS(-1), "sets the runtime GOMAXPROCS value")
	flag.BoolVar(&settings.EnableDryRun, "dryrun", false, "dry runs bot, doesn't save config file")
	flag.BoolVar(&settings.EnableAllExchanges, "enableallexchanges", false, "enables all exchanges")
	flag.BoolVar(&settings.EnableAllPairs, "enableallpairs", false, "enables all pairs for enabled exchanges")
	flag.BoolVar(&settings.EnablePortfolioManager, "portfoliomanager", true, "enables the portfolio manager")
	flag.BoolVar(&settings.EnableDataHistoryManager, "datahistorymanager", false, "enables the data history manager")
	flag.DurationVar(&settings.PortfolioManagerDelay, "portfoliomanagerdelay", 0, "sets the portfolio managers sleep delay between updates")
	flag.BoolVar(&settings.EnableGRPC, "grpc", true, "enables the grpc server")
	flag.BoolVar(&settings.EnableGRPCProxy, "grpcproxy", false, "enables the grpc proxy server")
	flag.BoolVar(&settings.EnableGRPCShutdown, "grpcshutdown", false, "enables gRPC bot instance shutdown functionality")
	flag.BoolVar(&settings.EnableWebsocketRPC, "websocketrpc", true, "enables the websocket RPC server")
	flag.BoolVar(&settings.EnableDeprecatedRPC, "deprecatedrpc", true, "enables the deprecated RPC server")
	flag.BoolVar(&settings.EnableCommsRelayer, "enablecommsrelayer", true, "enables available communications relayer")
	flag.BoolVar(&settings.Verbose, "verbose", true, "increases logging verbosity for GoCryptoTrader") // set to false for production
	flag.BoolVar(&settings.EnableFuturesTracking, "enablefuturestracking", true, "tracks futures orders PNL is supported by the exchange")
	flag.BoolVar(&settings.EnableExchangeSyncManager, "syncmanager", false, "enables to exchange sync manager")
	flag.BoolVar(&settings.EnableWebsocketRoutine, "websocketroutine", true, "enables the websocket routine for all loaded exchanges")
	flag.BoolVar(&settings.EnableCoinmarketcapAnalysis, "coinmarketcap", false, "overrides config and runs currency analysis")
	flag.BoolVar(&settings.EnableEventManager, "eventmanager", true, "enables the event manager")
	flag.BoolVar(&settings.EnableOrderManager, "ordermanager", true, "enables the order manager")
	flag.BoolVar(&settings.EnableDepositAddressManager, "depositaddressmanager", true, "enables the deposit address manager")
	flag.BoolVar(&settings.EnableConnectivityMonitor, "connectivitymonitor", true, "enables the connectivity monitor")
	flag.BoolVar(&settings.EnableDatabaseManager, "databasemanager", true, "enables database manager")
	flag.BoolVar(&settings.EnableGCTScriptManager, "gctscriptmanager", true, "enables gctscript manager")
	flag.DurationVar(&settings.EventManagerDelay, "eventmanagerdelay", 0, "sets the event managers sleep delay between event checking")
	flag.BoolVar(&settings.EnableNTPClient, "ntpclient", true, "enables the NTP client to check system clock drift")
	flag.BoolVar(&settings.EnableDispatcher, "dispatch", true, "enables the dispatch system")
	flag.BoolVar(&settings.EnableCurrencyStateManager, "currencystatemanager", true, "enables the currency state manager")
	flag.IntVar(&settings.DispatchMaxWorkerAmount, "dispatchworkers", dispatch.DefaultMaxWorkers, "sets the dispatch package max worker generation limit")
	flag.IntVar(&settings.DispatchJobsLimit, "dispatchjobslimit", dispatch.DefaultJobsLimit, "sets the dispatch package max jobs limit")

	// Exchange syncer settings
	flag.BoolVar(&settings.EnableTickerSyncing, "tickersync", false, "enables ticker syncing for all enabled exchanges, overriding false config value")
	flag.BoolVar(&settings.EnableOrderbookSyncing, "orderbooksync", false, "enables orderbook syncing for all enabled exchanges, overriding false config value")
	flag.BoolVar(&settings.EnableTradeSyncing, "tradesync", false, "enables trade syncing for all enabled exchanges, overriding false config value")
	flag.IntVar(&settings.SyncWorkersCount, "syncworkers", config.DefaultSyncerWorkers, "the amount of workers (goroutines) to use for syncing exchange data")
	flag.BoolVar(&settings.SyncContinuously, "synccontinuously", false, "whether to sync exchange data continuously (ticker, orderbook and trade history info), overriding false config value")
	flag.DurationVar(&settings.SyncTimeoutREST, "synctimeoutrest", config.DefaultSyncerTimeoutREST,
		"the amount of time before the syncer will switch from rest protocol to the streaming protocol (e.g. from REST to websocket)")
	flag.DurationVar(&settings.SyncTimeoutWebsocket, "synctimeoutwebsocket", config.DefaultSyncerTimeoutWebsocket,
		"the amount of time before the syncer will switch from the websocket protocol to REST protocol (e.g. from websocket to REST)")

	// Forex provider settings
	flag.BoolVar(&settings.EnableCurrencyConverter, "currencyconverter", false, "overrides config and sets up foreign exchange Currency Converter")
	flag.BoolVar(&settings.EnableCurrencyLayer, "currencylayer", false, "overrides config and sets up foreign exchange Currency Layer")
	flag.BoolVar(&settings.EnableExchangeRates, "exchangerates", false, "overrides config and sets up foreign exchange exchangeratesapi.io")
	flag.BoolVar(&settings.EnableFixer, "fixer", false, "overrides config and sets up foreign exchange Fixer.io")
	flag.BoolVar(&settings.EnableOpenExchangeRates, "openexchangerates", false, "overrides config and sets up foreign exchange Open Exchange Rates")

	// Exchange tuning settings
	flag.BoolVar(&settings.EnableExchangeAutoPairUpdates, "exchangeautopairupdates", false, "enables automatic available currency pair updates for supported exchanges")
	flag.BoolVar(&settings.DisableExchangeAutoPairUpdates, "exchangedisableautopairupdates", false, "disables exchange auto pair updates")
	flag.BoolVar(&settings.EnableExchangeWebsocketSupport, "exchangewebsocketsupport", false, "enables Websocket support for exchanges")
	flag.BoolVar(&settings.EnableExchangeRESTSupport, "exchangerestsupport", true, "enables REST support for exchanges")
	flag.BoolVar(&settings.EnableExchangeVerbose, "exchangeverbose", false, "increases exchange logging verbosity")
	flag.BoolVar(&settings.ExchangePurgeCredentials, "exchangepurgecredentials", false, "purges the stored exchange API credentials")
	flag.BoolVar(&settings.EnableExchangeHTTPRateLimiter, "ratelimiter", true, "enables the rate limiter for HTTP requests")
	flag.IntVar(&settings.RequestMaxRetryAttempts, "httpmaxretryattempts", request.DefaultMaxRetryAttempts, "sets the number of retry attempts after a retryable HTTP failure")
	flag.DurationVar(&settings.HTTPTimeout, "httptimeout", 0, "sets the HTTP timeout value for HTTP requests")
	flag.StringVar(&settings.HTTPUserAgent, "httpuseragent", "", "sets the HTTP user agent")
	flag.StringVar(&settings.HTTPProxy, "httpproxy", "", "sets the HTTP proxy server")
	flag.BoolVar(&settings.EnableExchangeHTTPDebugging, "exchangehttpdebugging", false, "sets the exchanges HTTP debugging")
	flag.DurationVar(&settings.TradeBufferProcessingInterval, "tradeprocessinginterval", trade.DefaultProcessorIntervalTime, "sets the interval to save trade buffer data to the database")
	flag.IntVar(&settings.AlertSystemPreAllocationCommsBuffer, "alertbuffer", alert.PreAllocCommsDefaultBuffer, "sets the size of the pre-allocation communications buffer")
	flag.DurationVar(&settings.ExchangeShutdownTimeout, "exchangeshutdowntimeout", time.Second*10, "sets the maximum amount of time the program will wait for an exchange to shut down gracefully")

	// Common tuning settings
	flag.DurationVar(&settings.GlobalHTTPTimeout, "globalhttptimeout", 0, "sets common HTTP timeout value for HTTP requests")
	flag.StringVar(&settings.GlobalHTTPUserAgent, "globalhttpuseragent", "", "sets the common HTTP client's user agent")
	flag.StringVar(&settings.GlobalHTTPProxy, "globalhttpproxy", "", "sets the common HTTP client's proxy server")

	// GCTScript tuning settings
	flag.UintVar(&settings.MaxVirtualMachines, "maxvirtualmachines", uint(gctscriptVM.DefaultMaxVirtualMachines), "set max virtual machines that can load")

	// Withdraw Cache tuning settings
	flag.Uint64Var(&settings.WithdrawCacheSize, "withdrawcachesize", withdraw.CacheSize, "set cache size for withdrawal requests")

	flag.Parse()

	if *versionFlag {
		fmt.Print(core.Version(true))
		os.Exit(0)
	}

	settings.CheckParamInteraction = true

	// collect flags
	flagSet := make(map[string]bool)
	// Stores the set flags
	flag.Visit(func(f *flag.Flag) { flagSet[f.Name] = true })
	if !flagSet["config"] {
		// If config file is not explicitly set, fall back to default path resolution
		settings.ConfigFile = ""
	}

	// gctscript setup in go routine
	go func() {
		gctscript.Setup()
	}()

	// base context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		interrupt := signaler.WaitForInterrupt()
		gctlog.Infof(gctlog.Global, "Captured %v, shutdown requested.\n", interrupt)
		cancel() // cancel the context to stop the engine and all its routines
	}()

	instance, err := delta.GetInstance(ctx, &settings, flagSet)
	if err != nil {
		gctlog.Errorf(gctlog.Global, "Failed to get instance: %v\n", err)
		os.Exit(1)
	}

	err = instance.StartEngine(ctx)
	if err != nil {
		gctlog.Errorf(gctlog.Global, "Failed to start engine: %v\n", err)
		os.Exit(1)
	}

	// initialize QuestDB repository
	questDBConfig := "http::addr=localhost:9000;"
	questDBRepo, err := repository.NewQuestDBRepository(ctx, questDBConfig)
	if err != nil {
		log.Fatalf("failed to create QuestDB repository: %v", err)
	}
	defer questDBRepo.Close(ctx)

	holdingsManager, err := delta.NewHoldingsManager(instance, questDBConfig)
	if holdingsErr := holdingsManager.UpdateHoldings(ctx, "bybit", asset.Spot); holdingsErr != nil {
		gctlog.Errorf(gctlog.Global, "Failed to update holdings: %v\n", holdingsErr)
		os.Exit(1)
	}

	time.Sleep(10 * time.Second)

	<-ctx.Done()
	fmt.Println("Shutdown in progress. This may take up to 30 seconds...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	done := make(chan struct{})
	var stopErr error

	go func() {
		fmt.Println("Stopping DeltaWorks engine...")
		stopErr = instance.StopEngine(shutdownCtx)
		close(done)
	}()

	// Setup a channel for second interrupt
	secondInterrupt := make(chan struct{}, 1)
	go func() {
		interrupt := signaler.WaitForInterrupt()
		fmt.Printf("\nSecond interrupt received (%v). Forcing immediate exit...\n", interrupt)
		gctlog.Infof(gctlog.Global, "Captured %v, second shutdown requested.\n", interrupt)
		secondInterrupt <- struct{}{}
	}()

	// Wait for either StopEngine to complete, context to be cancelled, or receive another interrupt
	select {
	case <-shutdownCtx.Done():
		if errors.Is(shutdownCtx.Err(), context.DeadlineExceeded) {
			gctlog.Errorln(gctlog.Global, "Shutdown timed out, forcing exit")
		} else {
			gctlog.Errorln(gctlog.Global, "Shutdown was cancelled, forcing exit")
		}
	case <-secondInterrupt:
		gctlog.Infoln(gctlog.Global, "Second interrupt received, forcing exit")
		shutdownCancel() // Cancel the shutdown context
	case <-done:
		if stopErr != nil {
			gctlog.Errorf(gctlog.Global, "Error during engine shutdown: %v\n", stopErr)
		} else {
			gctlog.Infoln(gctlog.Global, "Engine stopped successfully")
		}
	}

	// Wait for a short period to allow for any final cleanup
	fmt.Println("Performing final cleanup...")
	time.Sleep(9 * time.Second)

	fmt.Print("DeltaWorks has been shutdown gracefully")
	gctlog.Infof(gctlog.Global, "DeltaWorks has been shutdown gracefully\n")
}

//func insertData() error {
//	ctx := context.TODO()
//	sender, err := questdb.LineSenderFromConf(ctx, "http::addr=localhost:9000;")
//	if err != nil {
//		return fmt.Errorf("failed to create new line sender: %w", err)
//	}
//	defer sender.Close(ctx)
//
//	err = sender.
//		Table("holdings").
//		Symbol("exchange", "bybit").
//		Symbol("accountType", "spot").
//		Float64Column("amount", 0.7).
//		At(ctx, time.Now())
//	if err != nil {
//		return fmt.Errorf("failed to insert data: %w", err)
//	}
//
//	err = sender.Flush(ctx)
//	if err != nil {
//		return fmt.Errorf("failed to flush data: %w", err)
//	}
//
//	return nil
//}

func waitForInterrupt(cancel context.CancelFunc, waiter chan<- struct{}) {
	interrupt := signaler.WaitForInterrupt()
	gctlog.Infof(gctlog.Global, "Captured %v, shutdown requested.\n", interrupt)
	cancel() // cancel the context to stop the engine and all its routines
	waiter <- struct{}{}
}
