package handler

import (
	"fmt"
	"github.com/bincooo/chatgpt-adapter/v2/internal/middle"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
)

func Bind(port int, version, proxies string) {
	gin.SetMode(gin.ReleaseMode)
	route := gin.Default()

	route.Use(crosHandler)
	route.Use(panicHandler)
	route.Use(tokenHandler)
	route.Use(proxiesHandler(proxies))
	route.Use(func(ctx *gin.Context) {
		ctx.Set("port", port)
	})

	route.GET("/", welcome(version))
	route.POST("/hf/v1/chat/completions", completions)
	route.POST("/v1/object/completions", completions)
	route.POST("/proxies/v1/chat/completions", completions)
	route.POST("v1/images/generations", generations)
	route.POST("v1/object/generations", generations)
	route.POST("proxies/v1/images/generations", generations)
	route.GET("/proxies/v1/models", models)
	route.GET("/v1/models", models)
	route.Static("/file/tmp/", "tmp")

	addr := ":" + strconv.Itoa(port)
	logrus.Info(fmt.Sprintf("server start by http://0.0.0.0%s/v1", addr))
	if err := route.Run(addr); err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}

func proxiesHandler(proxies string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if proxies != "" {
			ctx.Set("proxies", proxies)
		}
	}
}

func tokenHandler(ctx *gin.Context) {
	token := ctx.Request.Header.Get("X-Api-Key")
	if token == "" {
		token = strings.TrimPrefix(ctx.Request.Header.Get("Authorization"), "Bearer ")
	}

	if token != "" {
		ctx.Set("token", token)
	}
}

func crosHandler(context *gin.Context) {
	method := context.Request.Method
	context.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	context.Header("Access-Control-Allow-Origin", "*") // 设置允许访问所有域
	context.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE,UPDATE")
	context.Header("Access-Control-Allow-Headers", "*")
	context.Header("Access-Control-Expose-Headers", "*")
	context.Header("Access-Control-Max-Age", "172800")
	context.Header("Access-Control-Allow-Credentials", "false")
	context.Set("content-type", "application/json")

	if method == "OPTIONS" {
		context.Status(http.StatusOK)
		return
	}

	uid := uuid.NewString()
	// 请求打印
	data, err := httputil.DumpRequest(context.Request, false)
	if err != nil {
		logrus.Error(err)
	} else {
		fmt.Printf("\n\n\n\n------ Start request %s  ---------\n%s\n", uid, data)
	}

	//处理请求
	context.Next()

	// 结束处理
	fmt.Printf("------ End request %s  ---------\n", uid)
}

func panicHandler(ctx *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("response error: %v", r)
			middle.ErrResponse(ctx, -1, fmt.Sprintf("%v", r))
		}
	}()

	//处理请求
	ctx.Next()
}

func welcome(version string) gin.HandlerFunc {
	return func(context *gin.Context) {
		w := context.Writer
		str := strings.ReplaceAll(html, "VERSION", version)
		str = strings.ReplaceAll(str, "HOST", context.Request.Host)
		_, _ = w.WriteString(str)
	}
}

func models(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"object": "list",
		"data":   GlobalExtension.Models(),
	})
}
