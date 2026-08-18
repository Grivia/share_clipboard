package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

const (
	maxPlaintextBytes = 256*1024 - 16
	maxPendingEvents  = 20
)

func (d *Daemon) queueLocalClipboard(text string) error {
	digest := contentDigest(text)
	if d.hasDigest && digest == d.lastDigest {
		return nil
	}
	d.lastDigest = digest
	d.hasDigest = true
	if len([]byte(text)) > maxPlaintextBytes {
		return fmt.Errorf("clipboard text exceeds 256 KiB")
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	upload, err := encryptText(text, d.runtime.SharedKey, eventID)
	if err != nil {
		return err
	}
	d.pending = append(d.pending, upload)
	if len(d.pending) > maxPendingEvents {
		d.pending = d.pending[len(d.pending)-maxPendingEvents:]
	}
	if err := d.stores.SavePending(d.pending); err != nil {
		return err
	}
	d.reporter.Update(func(status *DaemonStatus) { status.Pending = len(d.pending) })
	return nil
}

func (d *Daemon) flushPending(ctx context.Context) error {
	for len(d.pending) > 0 {
		upload := d.pending[0]
		var response ClipCreateResponse
		err := d.withAuth(ctx, func(token string) error {
			var requestErr error
			response, requestErr = d.api.Upload(ctx, token, upload)
			return requestErr
		})
		if err != nil {
			if apiError, ok := err.(*APIError); ok && apiError.Code == "CLIENT_EVENT_ID_REUSED" {
				log.Printf("discarding invalid pending event %s", upload.ClientEventID)
				d.pending = d.pending[1:]
				if saveErr := d.stores.SavePending(d.pending); saveErr != nil {
					return saveErr
				}
				continue
			}
			return err
		}
		_ = response
		d.pending = d.pending[1:]
		if err := d.stores.SavePending(d.pending); err != nil {
			return err
		}
		d.reporter.Update(func(status *DaemonStatus) { status.Pending = len(d.pending) })
	}
	return nil
}

func (d *Daemon) synchronize(ctx context.Context) error {
	cursor := d.runtime.LastSeq
	for {
		var response ClipsResponse
		err := d.withAuth(ctx, func(token string) error {
			var requestErr error
			response, requestErr = d.api.Clips(ctx, token, cursor)
			return requestErr
		})
		if err != nil {
			return err
		}

		for _, event := range response.Clips {
			if event.OriginDeviceID != d.runtime.DeviceID {
				text, err := decryptText(event, d.runtime.SharedKey)
				if err != nil {
					log.Printf("decrypt event %s: %v", event.EventID, err)
					d.reporter.Set("key_error", "Could not decrypt clipboard event; sign in again")
				} else {
					if err := d.bridge.Set(text); err != nil {
						return err
					}
					d.lastDigest = contentDigest(text)
					d.hasDigest = true
				}
			}
			if event.Seq > cursor {
				cursor = event.Seq
			}
		}

		if cursor > d.runtime.LastSeq {
			d.runtime.LastSeq = cursor
			if err := d.stores.SaveRuntime(d.runtime); err != nil {
				return err
			}
			if err := d.withAuth(ctx, func(token string) error {
				return d.api.Acknowledge(ctx, token, cursor)
			}); err != nil {
				return err
			}
		}
		if len(response.Clips) < 200 {
			break
		}
	}

	d.reporter.Update(func(status *DaemonStatus) {
		status.State = "ready"
		status.Message = "Synchronized"
		status.LastSyncAt = time.Now().UTC()
		status.Pending = len(d.pending)
		status.DeviceID = d.runtime.DeviceID
	})
	return nil
}
