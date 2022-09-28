package wiredtiger

var (
	defaultCreate        = true
	defaultCacheSize     = DiskSize{Size: 1, Type: GB}
	defaultCacheOverhead = 12
	defaultCheckpoint    = Checkpoint{LogSize: DiskSize{Size: 2, Type: GB}, Wait: 60}
	defaultConfigBase    = false
	defaultDebugMode     = DebugMode{CheckpointRetention: 0, CursorCopy: false, Eviction: false, TableLogging: false}
	defaultEviction      = Eviction{ThreadsMin: 4, ThreadsMax: 4}
	defaultFileManager   = FileManger{CloseIdleTime: 600, CloseScanInterval: 10, CloseHandleMinimum: 2000}
	defaultLog           = Log{Enabled: true, Archive: true, Path: "journal", Compressor: SnappyCompressor}
	defaultSessionMax    = 20
	defaultStatistics    = []StatisticsPolicy{StatisticsFast}
	defaultStatisticsLog = StatisticsLog{Wait: 0}
	defaultExtensions    = "[libwiredtiger_snappy.so]"
	defaultVerbose       = "[]"
)
