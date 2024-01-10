# 自定义 goctl-swagger 


### 1. goctl-swagger 修复分支

1. 修复：当存在第三方 tag 时，生成的参数名称错误的问题
2. 修复：当结构体嵌套超过2层时，参数不能正确生成的问题
3. 修复：当同时存在 form 和 json tag时，request body 包含多余字段的问题
4. 优化：支持从 [validate](https://github.com/go-playground/validator) tag 中获取参数约束
5. 升级 goctl 和 go-zero 到 v1.6.0，修复 [#71](https://github.com/zeromicro/goctl-swagger/issues/71)
6. 当请求方法是 POST/PUT/PATCH 时，如果请求字段不包含 json tag，且包含 form tag时，在请求的 content-type 中添加 "multipart/form-data", "application/x-www-form-urlencoded"
7. 添加 `-pack` 和 `-response` 选项，允许在 api 返回结构外再嵌套包装一层

### 2. 编译goctl-swagger插件

```bash
git clone https://github.com/sliveryou/goctl-swagger.git
cd goctl-swagger
go install
```

### 3. goctl-swagger 使用说明

使用例子：

```shell
# -filename 指定生成的 swagger 文件名称
# -pack 开启响应包装并指定响应结构名称
#  未指定 -response 时，默认使用如下响应结构

[{
	"name": "trace_id",
	"type": "string",
	"description": "链路追踪id"
}, {
	"name": "code",
	"type": "integer",
	"description": "状态码"
}, {
	"name": "msg",
	"type": "string",
	"description": "消息"
}, {
	"name": "data",
	"type": "object",
	"description": "数据",
	"is_data": true
}]

$ goctl api plugin -plugin goctl-swagger='swagger -filename rest.swagger.json -pack Response' -api api/base.api -dir api

# -filename 指定生成的 swagger 文件名称
# -pack 开启外层响应包装并指定外层响应结构名称
# -response 指定外层响应结构，需要进行转义，可以使用 https://www.bejson.com/ 进行转义，切记在最后的单引号 ' 前加上分号 ;
$ goctl api plugin -plugin goctl-swagger='swagger -filename rest.swagger.json -pack Response -response "[{\"name\":\"trace_id\",\"type\":\"string\",\"description\":\"链路追踪id\"},{\"name\":\"code\",\"type\":\"integer\",\"description\":\"状态码\"},{\"name\":\"msg\",\"type\":\"string\",\"description\":\"消息\"},{\"name\":\"data\",\"type\":\"object\",\"description\":\"数据\",\"is_data\":true}]";' -api api/base.api -dir api
```
