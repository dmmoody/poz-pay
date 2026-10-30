package postgres

import "github.com/dmmoody/poz-pay/internal/store"

var (
	_ store.Tenants         = (*Tenants)(nil)
	_ store.CheckRegister   = (*CheckRegister)(nil)
	_ store.CheckSettings   = (*CheckSettings)(nil)
	_ store.CheckAudit      = (*CheckAudit)(nil)
	_ store.RegisterUploads = (*RegisterUploads)(nil)
	_ store.UploadTx        = recordTx{}
	_ store.UploadConn      = recordConn{}
	_ store.Outbox          = (*Outbox)(nil)
)
