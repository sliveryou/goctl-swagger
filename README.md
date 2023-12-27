# 自定义 goctl-swagger 


### 1. goctl-swagger 修复分支

1. 修复：当存在第三方 tag 时，生成的参数名称错误的问题
2. 修复：当结构体嵌套超过2层时，参数不能正确生成的问题
3. 修复：当同时存在 form 和 json tag时，request body 包含多余字段的问题
4. 优化：支持从 [validate](https://github.com/go-playground/validator) tag 中获取参数约束
5. 升级 goctl 和 go-zero 到 v1.6.0，修复 [#71](https://github.com/zeromicro/goctl-swagger/issues/71)
6. 当请求方法是 POST/PUT/PATCH 时，如果请求字段不包含 json tag，且包含 form tag时，在请求的 content-type 中添加 "multipart/form-data", "application/x-www-form-urlencoded"

### 2. 编译goctl-swagger插件

```bash
git clone https://github.com/sliveryou/goctl-swagger.git
cd goctl-swagger
go install
```
