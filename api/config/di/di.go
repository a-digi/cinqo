package di

import (
	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
)

var _ serverdi.Context = (*ContextBag)(nil)

// ContextBag is the DI container passed to the route builder and, through
// it, to every request handler via reqCtx.GetDI().
type ContextBag struct {
	items           map[string]interface{}
	DatabaseManager *orm.DatabaseManager
	Logger          logger.Logger
}

func NewContextBag(manager *orm.DatabaseManager, log logger.Logger) *ContextBag {
	return &ContextBag{
		items:           make(map[string]interface{}),
		DatabaseManager: manager,
		Logger:          log,
	}
}

func (c *ContextBag) Get(key string) (interface{}, bool) {
	item, ok := c.items[key]
	return item, ok
}

func (c *ContextBag) Set(key string, value interface{}) {
	c.items[key] = value
}

func (c *ContextBag) GetDatabaseManager() *orm.DatabaseManager { return c.DatabaseManager }

func (c *ContextBag) GetLogger() logger.Logger { return c.Logger }
