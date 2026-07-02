// WITHOUT INTERFACES — everything calls the email package directly.
// Run: go run cmd/learn/without-interfaces/main.go
package main

import "fmt"

// ============================================================
// email.go — the notification provider (hardcoded)
// ============================================================

func emailSend(to string, message string) error {
	fmt.Printf("  [Email] Sending to %s: %s\n", to, message)
	return nil
}

// ============================================================
// order_service.go — places orders, notifies customer
// ============================================================

func placeOrder(customerEmail string, item string, total float64) error {
	fmt.Printf("Placing order: %s ($%.2f)\n", item, total)
	// directly calls emailSend — hardwired
	return emailSend(customerEmail, fmt.Sprintf("Order confirmed: %s for $%.2f", item, total))
}

// ============================================================
// refund_service.go — processes refunds, notifies customer
// ============================================================

func processRefund(customerEmail string, amount float64) error {
	fmt.Printf("Processing refund: $%.2f\n", amount)
	// directly calls emailSend — hardwired
	return emailSend(customerEmail, fmt.Sprintf("Refund of $%.2f processed", amount))
}

// ============================================================
// alert_service.go — sends admin alerts
// ============================================================

func sendAlert(adminEmail string, alert string) error {
	fmt.Printf("Triggering alert: %s\n", alert)
	// directly calls emailSend — hardwired
	return emailSend(adminEmail, fmt.Sprintf("ALERT: %s", alert))
}

// ============================================================
// main.go
// ============================================================

func main() {
	fmt.Println("=== WITHOUT INTERFACES ===")
	fmt.Println()

	_ = placeOrder("customer@example.com", "Widget", 29.99)
	fmt.Println()

	_ = processRefund("customer@example.com", 29.99)
	fmt.Println()

	_ = sendAlert("admin@example.com", "inventory low")
	fmt.Println()

	fmt.Println("Now imagine the boss says: switch from Email to SMS.")
	fmt.Println("You must change placeOrder, processRefund, AND sendAlert.")
	fmt.Println("That's 3 files. In a real project, it could be 20+.")
}
