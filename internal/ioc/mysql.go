package ioc

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Serendipity565/gora/configs"
	"github.com/Serendipity565/gora/internal/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitMysql 根据 cfg 初始化 MySQL 连接，并返回 *gorm.DB 实例。
func InitMysql(cfg configs.MySQLConfig) *gorm.DB {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		cfg.Username, cfg.Password, cfg.Addr, cfg.DBName)

	//logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	//if err != nil {
	//	panic(fmt.Sprintf("无法打开 MySQL 日志文件: %v", err))
	//}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold:             200 * time.Millisecond, // 慢 SQL 阈值
				LogLevel:                  logger.Warn,            // 日志级别
				IgnoreRecordNotFoundError: true,                   // 是否忽略记录未找到错误
				Colorful:                  false,                  // 是否彩色打印
			},
		),
	})
	if err != nil {
		panic(fmt.Sprintf("Mysql 连接失败: %v", err))
	}

	// 获取底层 *sql.DB
	sqlDB, err := db.DB()
	if err != nil {
		panic(fmt.Sprintf("获取 sql.DB 失败: %v", err))
	}

	// 最大打开连接数
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	// 最大空闲连接数
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	// 连接最大生命周期
	if cfg.ConnMaxLifetime != "" {
		lifetime, err := time.ParseDuration(cfg.ConnMaxLifetime)
		if err != nil {
			panic(fmt.Sprintf(
				"conn_max_lifetime 配置错误: %v",
				err,
			))
		}
		sqlDB.SetConnMaxLifetime(lifetime)
	}

	// 自动迁移业务表结构。
	err = repository.InitTables(db)
	if err != nil {
		panic(fmt.Sprintf("MySQL 自动迁移失败: %v", err))
	}

	return db
}
