module main

go 1.13

require github.com/colonio/colonio-seed/src/seed v0.0.0-00010101000000-000000000000 // indirect

replace (
	github.com/colonio/colonio-seed/src/proto => ./proto
	github.com/colonio/colonio-seed/src/seed => ./seed
)
