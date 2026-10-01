package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hirotomasato/scbz/sociabuzz"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: scbz-test <creator-username>")
		os.Exit(1)
	}
	username := os.Args[1]
	ctx := context.Background()

	// Test fee estimate (read-only, no actual donation)
	fmt.Println("=== Fee Estimate: Malaysia MYR 5.0 ===")
	calc := sociabuzz.NewFeeCalculator("", username)
	estimates, err := calc.Estimate(ctx, "Malaysia", 5.0, username)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fee estimate failed: %v\n", err)
		os.Exit(1)
	}
	for _, e := range estimates {
		fmt.Printf("  %-20s fee=%s pay=%s (gateway=%s)\n", e.Label, e.Fee.TotalFee, e.Fee.TotalPay, e.Gateway)
	}
	calc.Close()

	fmt.Println("\n=== Donation Flow: Malaysia MYR 5.0 ===")
	sb := sociabuzz.New("")
	donation, err := sb.Donate(ctx, username, 5.0, "Malaysia", "Test Donor", "test@example.com", "SDK smoke test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Donate failed: %v\n", err)
		os.Exit(1)
	}
	defer donation.Close()

	fmt.Printf("Token: %s\n", donation.Token)
	fmt.Printf("Redirect URL: %s\n", donation.RedirectURL)
	fmt.Printf("Amount: %.2f %s\n", donation.Amount, donation.Currency)

	methods, err := donation.GetPaymentMethods(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetPaymentMethods failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Available methods: %d\n", len(methods))
	for _, m := range methods {
		fmt.Printf("  #%d: %-20s (gateway=%s)\n", m.ID, m.Label, m.Gateway)
	}

	if len(methods) > 0 {
		firstMethod := methods[0].Name
		fmt.Printf("\n=== Creating Payment: %s ===\n", firstMethod)
		payment, err := donation.CreatePayment(ctx, firstMethod)
		if err != nil {
			fmt.Fprintf(os.Stderr, "CreatePayment failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Method: %s (%s)\n", payment.Label, payment.Gateway)
		fmt.Printf("Payment URL: %s\n", payment.PaymentURL)
		if payment.QRString != "" {
			fmt.Printf("QR string: %s\n", payment.QRString)
		}
	}

	fmt.Println("\n=== Done ===")
}