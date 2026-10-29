package controller

import (
	"net/url"
	"strconv"

	"ops-admin/backend/httpx"
	"ops-admin/backend/service"

	"github.com/gin-gonic/gin"
)

func (ctl *Controller) GetAssetDatabaseList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	data, err := ctl.service.ListAssetDatabases(pageNum, pageSize, c.Query("keyword"), c.Query("dbType"), c.Query("status"), c.Query("env"), c.Query("tag"))
	if err != nil {
		httpx.Failed(c, 500, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetDatabaseInfo(c *gin.Context) {
	data, err := ctl.service.GetAssetDatabase(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 404, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetAssetDatabaseMetrics(c *gin.Context) {
	data, err := ctl.service.GetAssetDatabaseMetrics(uint(mustAtoi(c.Query("id"))), c.DefaultQuery("range", "1h"), c.Query("start"), c.Query("end"))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateAssetDatabase(c *gin.Context) {
	var payload service.AssetDatabasePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid database payload")
		return
	}
	payload.Operator = c.GetString("username")
	if err := ctl.service.CreateAssetDatabase(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) UpdateAssetDatabase(c *gin.Context) {
	var payload service.AssetDatabasePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid database payload")
		return
	}
	payload.Operator = c.GetString("username")
	if err := ctl.service.UpdateAssetDatabase(payload); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) DeleteAssetDatabase(c *gin.Context) {
	var payload service.IDPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid delete payload")
		return
	}
	if err := ctl.service.DeleteAssetDatabase(payload.ID); err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, true)
}

func (ctl *Controller) TestAssetDatabaseConnection(c *gin.Context) {
	var payload service.AssetDatabasePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid database payload")
		return
	}
	data, err := ctl.service.TestAssetDatabaseConnection(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetDatabaseWorkbench(c *gin.Context) {
	data, err := ctl.service.GetDatabaseWorkbench(uint(mustAtoi(c.Query("databaseId"))))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetDatabaseSchemaTree(c *gin.Context) {
	data, err := ctl.service.GetDatabaseSchemaTree(uint(mustAtoi(c.Query("databaseId"))))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateDatabaseSchema(c *gin.Context) {
	var payload service.DBMSCreateDatabasePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid database create payload")
		return
	}
	payload.Operator = c.GetString("username")
	data, err := ctl.service.CreateDatabaseSchema(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetDatabaseCharsetOptions(c *gin.Context) {
	data, err := ctl.service.GetDatabaseCharsetOptions(uint(mustAtoi(c.Query("databaseId"))), c.Query("charset"))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetDatabaseTableData(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "25"))
	data, err := ctl.service.GetDatabaseTableData(service.DBMSTableDataQueryPayload{
		DatabaseID: uint(mustAtoi(c.Query("databaseId"))),
		Schema:     c.Query("schema"),
		Table:      c.Query("table"),
		PageNum:    pageNum,
		PageSize:   pageSize,
		FilterKey:  c.Query("filterKey"),
		FilterText: c.Query("filterText"),
	})
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetDatabaseResourceData(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "25"))
	data, err := ctl.service.GetDatabaseResourceData(service.DBMSTableDataQueryPayload{
		DatabaseID: uint(mustAtoi(c.Query("databaseId"))),
		Schema:     c.Query("schema"),
		Table:      c.Query("table"),
		PageNum:    pageNum,
		PageSize:   pageSize,
		FilterKey:  c.Query("filterKey"),
		FilterText: c.Query("filterText"),
	})
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ExecuteDatabaseSQL(c *gin.Context) {
	var payload service.DBMSSQLExecutePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid sql payload")
		return
	}
	payload.Operator = c.GetString("username")
	payload.ClientIP = c.ClientIP()
	data, err := ctl.service.ExecuteDatabaseSQL(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) AnalyzeDatabaseSQL(c *gin.Context) {
	var payload service.DBMSSQLExecutePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid sql payload")
		return
	}
	data, err := ctl.service.AnalyzeDatabaseSQL(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) AnalyzeRedisCommand(c *gin.Context) {
	var payload service.RedisCommandPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "Redis 命令参数不正确")
		return
	}
	data, err := ctl.service.AnalyzeRedisCommand(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetRedisKeys(c *gin.Context) {
	data, err := ctl.service.GetRedisKeys(service.RedisKeysQuery{DatabaseID: uint(mustAtoi(c.Query("databaseId"))), Schema: c.Query("schema"), Cursor: c.Query("cursor"), Pattern: c.Query("pattern")})
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) ExecuteRedisCommand(c *gin.Context) {
	var payload service.RedisCommandPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "Redis 命令参数不正确")
		return
	}
	payload.Operator = c.GetString("username")
	payload.ClientIP = c.ClientIP()
	data, err := ctl.service.ExecuteRedisCommand(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetDatabaseSQLHistory(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	data, err := ctl.service.ListDatabaseSQLHistory(uint(mustAtoi(c.Query("databaseId"))), pageNum, pageSize)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) InsertDatabaseTableRow(c *gin.Context) {
	var payload service.DBMSTableInsertPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid row payload")
		return
	}
	data, err := ctl.service.InsertDatabaseTableRow(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) UpdateDatabaseTableRow(c *gin.Context) {
	var payload service.DBMSTableUpdatePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid row payload")
		return
	}
	data, err := ctl.service.UpdateDatabaseTableRow(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DeleteDatabaseTableRow(c *gin.Context) {
	var payload service.DBMSTableDeletePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid row payload")
		return
	}
	data, err := ctl.service.DeleteDatabaseTableRow(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateExportTask(c *gin.Context) {
	var payload service.DBMSExportPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid export payload")
		return
	}
	data, err := ctl.service.CreateExportTask(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateImportTask(c *gin.Context) {
	var payload service.DBMSImportPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid import payload")
		return
	}
	data, err := ctl.service.CreateImportTask(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) PrecheckImportTask(c *gin.Context) {
	var payload service.DBMSImportPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid import payload")
		return
	}
	data, err := ctl.service.PrecheckImportTask(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) CreateBatchSQLTask(c *gin.Context) {
	var payload service.DBMSBatchSQLPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Failed(c, 400, "invalid batch sql payload")
		return
	}
	payload.Operator = c.GetString("username")
	payload.ClientIP = c.ClientIP()
	data, err := ctl.service.CreateBatchSQLTask(payload)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) GetTransferTaskList(c *gin.Context) {
	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	data, err := ctl.service.ListTransferTasks(uint(mustAtoi(c.Query("databaseId"))), c.Query("taskType"), pageNum, pageSize)
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	httpx.Success(c, data)
}

func (ctl *Controller) DownloadTransferTask(c *gin.Context) {
	data, filename, err := ctl.service.GetTransferTaskFile(uint(mustAtoi(c.Query("id"))))
	if err != nil {
		httpx.Failed(c, 400, err.Error())
		return
	}
	c.Header("Content-Type", "application/sql")
	c.Header("Content-Disposition", "attachment; filename="+url.QueryEscape(filename))
	c.Data(200, "application/sql", data)
}
