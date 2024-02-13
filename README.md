# 自定义 goctl-swagger 

### 1. goctl-swagger 修复分支

1. 修复：当存在第三方 tag 时，生成的参数名称错误的问题
2. 修复：当结构体嵌套超过2层时，参数不能正确生成的问题
3. 修复：当同时存在 form 和 json tag 时，request body 包含多余字段的问题
4. 优化：支持从 [validate](https://github.com/go-playground/validator) tag 中获取参数约束
5. 修复：升级 goctl 和 go-zero 到 v1.6.1，修复 [#71](https://github.com/zeromicro/goctl-swagger/issues/71)
6. 添加：当请求方法是 POST/PUT/PATCH 时，如果请求字段不包含 json tag，且包含 form tag 时，在请求的 content-type 中添加 "multipart/form-data", "application/x-www-form-urlencoded"
7. 添加：`-pack` 和 `-response` 选项，允许在 api 返回结构外再嵌套包装一层
8. 修复：当结构体嵌套超过2层时，不能继承内联结构体属性的问题
9. 优化：支持根据 `@doc()` 里的 `file_*` 或 `file_array_*` 键值生成文件类型的请求字段
10. 添加：`delete` 请求方式允许携带请求体

### 2. 编译 goctl-swagger 插件

**需要 go 1.19 以上版本编译安装**

```bash
# 可选：自行编译安装：
$ git clone https://github.com/sliveryou/goctl-swagger.git
$ cd goctl-swagger
$ go install

# 推荐：使用如下命令安装：
$ go install github.com/sliveryou/goctl-swagger@latest
```

### 3. goctl-swagger 使用说明

在 api 返回结构外再嵌套包装一层：

```bash
# -filename 指定生成的 swagger 文件名称
# -pack 开启响应包装并指定响应结构名称
#  未指定 -response 时，默认使用如下响应结构，其中 is_data 字段为指定响应结构的包装字段

[{
	"name": "trace_id",
	"type": "string",
	"description": "链路追踪id",
	"example": "a1b2c3d4e5f6g7h8"
}, {
	"name": "code",
	"type": "integer",
	"description": "状态码",
	"example": 0
}, {
	"name": "msg",
	"type": "string",
	"description": "消息",
	"example": "ok"
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

支持根据 `@doc()` 里的 `file_*` 或 `file_array_*` 键值生成文件类型的请求字段：

```
解析 api 文件中的 @doc 中的 "file_*" 或 "file_array_*" 键值
"*" 表示文件字段名，如下所示：

service xxxx {
    @doc (
        file_upload: "false, 上传文件"
        file_array_upload: "false, 上传文件数组"
    )
    @handler xxxx
    ......
}

其属性用逗号分隔，第一个代表文件是否必填，第二个表示文件的描述
```
