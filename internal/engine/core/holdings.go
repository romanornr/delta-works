package core

//func (i *Instance) DisplayHoldings(exchange string) error {
//	// Engine getExchange by name and display holdings
//	// c.engine.GetExchangeByName("binance").GetHoldings()
//	// but we need a good variable name for the getExchangeByName function
//	x, err := i.Engine.GetExchangeByName(exchange)
//	if err != nil {
//		return fmt.Errorf("failed to get exchange by name: %v", err)
//	}
//
//	ctx := context.Background()
//	holdings, err := x.FetchAccountInfo(ctx, asset.Spot)
//	if err != nil {
//		return fmt.Errorf("failed to fetch account info: %v", err)
//	}
//
//	fmt.Printf("Holdings for %s:\n", exchange)
//	for _, account := range holdings.Accounts {
//
//		fmt.Printf("Account ID: %s\n", account.ID)
//		for _, currency := range account.Currencies {
//			fmt.Printf("Currency: %s, Total: %f, Hold: %f, Available: %f\n",
//				currency.Currency,
//				currency.Total,
//				currency.Hold,
//				currency.AvailableWithoutBorrow)
//		}
//	}
//
//	return nil
//}
