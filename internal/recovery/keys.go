package recovery

import (
	"fmt"
	"io"
	"os"

	"filippo.io/age"
)

// InitializeArchiveIdentity writes a new archive-only identity into a private
// operator-selected file. The returned value is public; private material is
// never printed or included in the database or capture manifest.
func InitializeArchiveIdentity(identityPath, recipientPath string) (string, error) {
	if identityPath == "" || recipientPath == "" || identityPath == recipientPath {
		return "", ErrArchive
	}
	identity, err := age.GenerateHybridIdentity()
	if err != nil {
		return "", fmt.Errorf("generate archive identity: %w", err)
	}
	public := identity.Recipient().String()
	if err = writeExclusive(recipientPath, public+"\n"); err != nil {
		return "", err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(recipientPath)
		}
	}()
	if err = writeExclusive(identityPath, identity.String()+"\n"); err != nil {
		return "", err
	}
	complete = true
	return public, nil
}

func writeExclusive(path, value string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create private archive key file: %w", err)
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(path)
		}
	}()
	if _, err = io.WriteString(file, value); err != nil {
		return fmt.Errorf("write private archive key file: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("sync private archive key file: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close private archive key file: %w", err)
	}
	complete = true
	return nil
}
