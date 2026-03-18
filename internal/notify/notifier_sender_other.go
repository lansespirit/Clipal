//go:build !darwin

package notify

func platformSender(sender func(title, message string, icon any) error) func(title, message string, icon any) error {
	return sender
}
