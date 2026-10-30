package store

var (
	_ Tenants         = (*tenants)(nil)
	_ CheckRegister   = (*checkRegister)(nil)
	_ CheckSettings   = (*checkSettings)(nil)
	_ CheckAudit      = (*checkAudit)(nil)
	_ RegisterUploads = (*registerUploads)(nil)
	_ UploadTx        = recordTx{}
	_ UploadConn      = recordConn{}
	_ Outbox          = (*outbox)(nil)
)
