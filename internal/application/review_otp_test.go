package application

import "testing"

func TestNewReviewOTPUsesBroadSixDigitSpace(t *testing.T) {
	seen := make(map[string]struct{}, 512)
	for range 512 {
		otp, err := newReviewOTP()
		if err != nil {
			t.Fatal(err)
		}
		if len(otp) != 6 {
			t.Fatalf("expected six digits, got %q", otp)
		}
		for _, digit := range otp {
			if digit < '0' || digit > '9' {
				t.Fatalf("expected numeric OTP, got %q", otp)
			}
		}
		seen[otp] = struct{}{}
	}
	if len(seen) < 256 {
		t.Fatalf("OTP generator has insufficient entropy: only %d unique values in 512 samples", len(seen))
	}
}
