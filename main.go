package main

import (
	"flag"
	"fmt"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/config"
	"github.com/thrasher-corp/gocryptotrader/core"
	"github.com/thrasher-corp/gocryptotrader/dispatch"
	"github.com/thrasher-corp/gocryptotrader/engine"
	"github.com/thrasher-corp/gocryptotrader/exchanges/alert"
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
	flag.BoolVar(&settings.Verbose, "verbose", false, "increases logging verbosity for GoCryptoTrader")
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

	fmt.Print(core.Banner)
	fmt.Println(core.Version(false))

	var err error
	settings.CheckParamInteraction = true

	// collect flags
	flagSet := make(map[string]bool)
	// Stores the set flags
	flag.Visit(func(f *flag.Flag) { flagSet[f.Name] = true })
	if !flagSet["config"] {
		// If config file is not explicitly set, fall back to default path resolution
		settings.ConfigFile = ""
	}

	settings.Shutdown = make(chan struct{})
	engine.Bot, err = engine.NewFromSettings(&settings, flagSet)
	if engine.Bot == nil || err != nil {
		log.Fatalf("Unable to initialise bot engine. Error: %s\n", err)
	}
	config.SetConfig(engine.Bot.Config)

	gctscript.Setup()

	engine.Bot.Settings.PrintLoadedSettings()

	if err = engine.Bot.Start(); err != nil {
		errClose := gctlog.CloseLogger()
		if errClose != nil {
			log.Printf("Unable to close logger. Error: %s\n", errClose)
		}
		log.Fatalf("Unable to start bot engine. Error: %s\n", err)
	}

	go waitForInterrupt(settings.Shutdown)
	<-settings.Shutdown
	engine.Bot.Stop()
}

func waitForInterrupt(waiter chan<- struct{}) {
	interrupt := signaler.WaitForInterrupt()
	gctlog.Infof(gctlog.Global, "Captured %v, shutdown requested.\n", interrupt)
	waiter <- struct{}{}
}

//func main() {
//	if err := initLogger(); err != nil {
//		log.Fatalf("Failed to initialise logger. Error: %s", err)
//	}
//
//	// Load the configuration file
//	configFile := util.ConfigFile(DefaultConfigPath)
//
//	app, err := NewBotApplication(&engine.Settings{ConfigFile: configFile})
//	if err != nil {
//		log.Fatalf("Failed to create bot application. Error: %s", err)
//		return // Graceful exit on failure to create bot application
//	}
//
//	if app.Bot == nil || err != nil {
//		log.Printf("Unable to create bot application. Error: %s\n", err)
//	}
//
//	if err := app.Start(); err != nil {
//		log.Fatalf("Failed to start bot application. Error: %s", err)
//		return // Graceful exit on failure to start bot application
//	}
//
//	//settings.Shutdown = make(chan struct{})
//	//engine.Bot, err = engine.NewFromSettings(&settings, flagSet)
//	//if engine.Bot == nil || err != nil {
//	//	log.Fatalf("Unable to initialise bot engine. Error: %s\n", err)
//	//}
//
//	gctscript.Setup()
//
//	app.Bot.Settings.PrintLoadedSettings()
//
//	if err = app.Bot.Start(); err != nil {
//		errClose := gctlog.CloseLogger()
//		if errClose != nil {
//			log.Printf("Unable to close logger. Error: %s\n", errClose)
//		}
//		log.Fatalf("Unable to start bot engine. Error: %s\n", err)
//	}
//
//	//go app.Run(context.Background())
//
//	//defer app.Stop()
//	go signaler.WaitForInterrupt()
//}
//
//func initLogger() error {
//	var err error
//	if err == gctlog.SetupGlobalLogger("Delta-Works", true) {
//		return err
//	}
//	gctlog.Debugf(gctlog.Global, "Logger initialised.")
//	return nil
//}
//
//type BotApplication struct {
//	Bot *engine.Engine
//}
//
//// NewBotApplication creates a new bot application
//func NewBotApplication(settings *engine.Settings) (*BotApplication, error) {
//	bot, err := engine.NewFromSettings(settings, nil)
//	if err != nil {
//		return nil, err
//	}
//	return &BotApplication{Bot: bot}, nil
//}
//
//// Start starts the bot application
//func (b *BotApplication) Start() error {
//	b.Bot.Settings.PrintLoadedSettings()
//	if err := b.Bot.Start(); err != nil {
//		// Attempt to close the logger gracefully
//		if errClose := gctlog.CloseLogger(); errClose != nil {
//			log.Fatalf("Failed to close logger. Error: %s", errClose)
//		}
//		return err
//	}
//	return nil
//}
//
//// Stop stops the bot application
//func (b *BotApplication) Stop() {
//	b.Bot.Stop()
//}
//
//func (b *BotApplication) Run(ctx context.Context) {
//	//var wg sync.WaitGroup
//	//exchanges, err := b.Bot.ExchangeManager.GetExchanges()
//	//if err != nil {
//	//	log.Fatalf("Failed to get exchanges. Error: %s", err)
//	//}
//
//	gctscript.Setup()
//
//	//for _, x := range exchanges {
//	//	wg.Add(1)
//	//
//	//	go func(x exchange.IBotExchange) {
//	//		defer wg.Done()
//	//
//	//		err := Loop(ctx, b, x)
//	//
//	//		panic(err)
//	//	}(x)
//	//}
//	//wg.Wait()
//
//	go signaler.WaitForInterrupt()
//	engine.Bot.Stop()
//}
//
//func Loop(ctx context.Context, b *BotApplication, e exchange.IBotExchange) error {
//	if !e.IsWebsocketEnabled() {
//		log.Fatalf("Websocket not enabled for exchange: %s", e.GetName())
//		<-ctx.Done()
//		return nil
//	}
//	return stream.Stream(ctx, e)
//}

//
//func (b *BotApplication) Sub() {
//	e, err := b.Bot.GetExchangeByName("bybit")
//	if err != nil {
//		fmt.Printf("Failed to get exchange. Error: %s", err)
//	}
//
//	logrus.Infof("Exchange: %s", e.GetName())
//	if e.SupportsWebsocket() {
//		wsInterface, err := e.GetWebsocket()
//		if err != nil {
//			fmt.Printf("Failed to get websocket. Error: %s", err)
//			return
//		}
//		subscribeToTicker(wsInterface)
//	}
//
//}
//
//func subscribeToTicker(wsInterface *stream.Websocket) {
//	subscriptions := []subscription.Subscription{}
//	subscriptions = append(subscriptions, subscription.Subscription{
//		Channel: subscription.TickerChannel,
//	})
//
//	err := wsInterface.SubscribeToChannels(subscriptions)
//	if err != nil {
//		fmt.Printf("Failed to subscribe to ticker. Error: %s", err)
//	}
//
//	wsInterface.GetSubscription()
//}
