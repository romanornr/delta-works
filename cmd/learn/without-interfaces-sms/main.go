// WITHOUT INTERFACES — AFTER switching from Email to SMS.
// Run: go run cmd/learn/without-interfaces-sms/main.go
//
// Compare this with without-interfaces/main.go.
// Every function that called emailSend had to be found and changed.
// Changes marked with // ← CHANGED
package main

import "fmt"

// ============================================================
// sms.go — the NEW notification provider
// ============================================================

func smsSend(to string, message string) error { // ← NEW (replaces emailSend)
	fmt.Printf("  [SMS] Texting %s: %s\n", to, message)
	return nil
}

// ============================================================
// order_service.go — HAD TO CHANGE
// ============================================================

func placeOrder(customerPhone string, item string, total float64) error {
	fmt.Printf("Placing order: %s ($%.2f)\n", item, total)
	return smsSend(customerPhone, fmt.Sprintf("Order confirmed: %s for $%.2f", item, total)) // ← CHANGED from emailSend
}

// ============================================================
// refund_service.go — HAD TO CHANGE
// ============================================================

func processRefund(customerPhone string, amount float64) error {
	fmt.Printf("Processing refund: $%.2f\n", amount)
	return smsSend(customerPhone, fmt.Sprintf("Refund of $%.2f processed", amount)) // ← CHANGED from emailSend
}

// ============================================================
// alert_service.go — HAD TO CHANGE
// ============================================================

func sendAlert(adminPhone string, alert string) error {
	fmt.Printf("Triggering alert: %s\n", alert)
	return smsSend(adminPhone, fmt.Sprintf("ALERT: %s", alert)) // ← CHANGED from emailSend
}

// ============================================================
// main.go — also changed
// ============================================================

func main() {
	fmt.Println("=== WITHOUT INTERFACES (switched to SMS) ===")
	fmt.Println()
	fmt.Println("Files changed: sms.go (new), order_service.go, refund_service.go, alert_service.go, main.go")
	fmt.Println("Total files touched: 5")
	fmt.Println()

	_ = placeOrder("+31612345678", "Widget", 29.99)
	fmt.Println()

	_ = processRefund("+31612345678", 29.99)
	fmt.Println()

	_ = sendAlert("+31687654321", "inventory low")
}
