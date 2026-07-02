// WITH INTERFACES — AFTER switching from Email to SMS.
// Run: go run cmd/learn/with-interfaces-sms/main.go
//
// Compare this with with-interfaces/main.go.
// ONLY main.go and the new smsNotifier changed.
// OrderService, RefundService, AlertService — ZERO changes.
// Changes marked with // ← CHANGED or // ← NEW
package main

import "fmt"

// ============================================================
// notifier.go — the interface (UNCHANGED)
// ============================================================

type Notifier interface {
	Send(to string, message string) error
}

// ============================================================
// sms.go — NEW implementation of Notifier
// ============================================================

type smsNotifier struct{} // ← NEW (this is the only new code)

func (s *smsNotifier) Send(to string, message string) error { // ← NEW
	fmt.Printf("  [SMS] Texting %s: %s\n", to, message)
	return nil
}

// ============================================================
// order_service.go — UNCHANGED (identical to with-interfaces/main.go)
// ============================================================

type OrderService struct {
	notify Notifier
}

func (svc *OrderService) PlaceOrder(customerContact string, item string, total float64) error {
	fmt.Printf("Placing order: %s ($%.2f)\n", item, total)
	return svc.notify.Send(customerContact, fmt.Sprintf("Order confirmed: %s for $%.2f", item, total))
}

// ============================================================
// refund_service.go — UNCHANGED (identical to with-interfaces/main.go)
// ============================================================

type RefundService struct {
	notify Notifier
}

func (svc *RefundService) ProcessRefund(customerContact string, amount float64) error {
	fmt.Printf("Processing refund: $%.2f\n", amount)
	return svc.notify.Send(customerContact, fmt.Sprintf("Refund of $%.2f processed", amount))
}

// ============================================================
// alert_service.go — UNCHANGED (identical to with-interfaces/main.go)
// ============================================================

type AlertService struct {
	notify Notifier
}

func (svc *AlertService) SendAlert(adminContact string, alert string) error {
	fmt.Printf("Triggering alert: %s\n", alert)
	return svc.notify.Send(adminContact, fmt.Sprintf("ALERT: %s", alert))
}

// ============================================================
// main.go — ONE line changed
// ============================================================

func main() {
	fmt.Println("=== WITH INTERFACES (switched to SMS) ===")
	fmt.Println()
	fmt.Println("Files changed: sms.go (new), main.go (1 line)")
	fmt.Println("Files UNCHANGED: order_service.go, refund_service.go, alert_service.go")
	fmt.Println("Total files touched: 2 (vs 5 without interfaces)")
	fmt.Println()

	// THIS is the only line that changed vs with-interfaces/main.go:
	notifier := &smsNotifier{} // ← CHANGED from &emailNotifier{}

	// Everything below is IDENTICAL to with-interfaces/main.go:
	orders := &OrderService{notify: notifier}
	refunds := &RefundService{notify: notifier}
	alerts := &AlertService{notify: notifier}

	_ = orders.PlaceOrder("+31612345678", "Widget", 29.99)
	fmt.Println()

	_ = refunds.ProcessRefund("+31612345678", 29.99)
	fmt.Println()

	_ = alerts.SendAlert("+31687654321", "inventory low")
}
