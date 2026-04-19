// WITH INTERFACES — services depend on a Notifier interface, not email directly.
// Run: go run cmd/learn/with-interfaces/main.go
package main

import "fmt"

// ============================================================
// notifier.go — the interface (the contract / the "box label")
// ============================================================

type Notifier interface {
	Send(to string, message string) error
}

// ============================================================
// email.go — one implementation of Notifier
// ============================================================

type emailNotifier struct{}

func (e *emailNotifier) Send(to string, message string) error {
	fmt.Printf("  [Email] Sending to %s: %s\n", to, message)
	return nil
}

// ============================================================
// order_service.go — places orders, notifies customer
// Uses Notifier interface — does NOT know about email
// ============================================================

type OrderService struct {
	notify Notifier // could be email, SMS, anything
}

func (svc *OrderService) PlaceOrder(customerContact string, item string, total float64) error {
	fmt.Printf("Placing order: %s ($%.2f)\n", item, total)
	return svc.notify.Send(customerContact, fmt.Sprintf("Order confirmed: %s for $%.2f", item, total))
}

// ============================================================
// refund_service.go — processes refunds, notifies customer
// Uses Notifier interface — does NOT know about email
// ============================================================

type RefundService struct {
	notify Notifier
}

func (svc *RefundService) ProcessRefund(customerContact string, amount float64) error {
	fmt.Printf("Processing refund: $%.2f\n", amount)
	return svc.notify.Send(customerContact, fmt.Sprintf("Refund of $%.2f processed", amount))
}

// ============================================================
// alert_service.go — sends admin alerts
// Uses Notifier interface — does NOT know about email
// ============================================================

type AlertService struct {
	notify Notifier
}

func (svc *AlertService) SendAlert(adminContact string, alert string) error {
	fmt.Printf("Triggering alert: %s\n", alert)
	return svc.notify.Send(adminContact, fmt.Sprintf("ALERT: %s", alert))
}

// ============================================================
// main.go — the ONE place that decides which Notifier to use
// ============================================================

func main() {
	fmt.Println("=== WITH INTERFACES (using Email) ===")
	fmt.Println()

	// Create the notifier — this is the ONLY place that knows about email
	notifier := &emailNotifier{}

	// All services receive the notifier — they don't know it's email
	orders := &OrderService{notify: notifier}
	refunds := &RefundService{notify: notifier}
	alerts := &AlertService{notify: notifier}

	_ = orders.PlaceOrder("customer@example.com", "Widget", 29.99)
	fmt.Println()

	_ = refunds.ProcessRefund("customer@example.com", 29.99)
	fmt.Println()

	_ = alerts.SendAlert("admin@example.com", "inventory low")
	fmt.Println()

	fmt.Println("Now imagine the boss says: switch from Email to SMS.")
	fmt.Println("See with-interfaces-sms/main.go for what changes.")
	fmt.Println("Spoiler: OrderService, RefundService, AlertService — ZERO changes.")
}
