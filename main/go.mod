module github.com/colonio/colonio-seed/main

go 1.12

require (
	github.com/colonio/colonio-seed/seed v0.0.0
	github.com/gobwas/ws v1.0.0
	github.com/mailru/easygo v0.0.0-20180630075556-0c8322a753d0
)

replace github.com/colonio/colonio-seed/seed => ../seed

replace github.com/colonio/colonio-seed/proto => ../proto
