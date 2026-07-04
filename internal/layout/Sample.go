package layout

// SampleContents 是覆盖各元素的示例小票排版(基于云端 JSON 排版结构),供「打印样票」验证。
// 结构与 MQTT type=5 的 contents 一致;png 用示例 URL,不可达则自动跳过。
var SampleContents = []byte(`[
  {"cont":"聪明付 排版样票","type":"title"},
  {"cont":"普通文本行","type":"text"},
  {"cont":"加粗文本行","type":"text","bold":true},
  {"cont":"放大1倍","type":"text","size":"11"},
  {"cont":"放大2倍","type":"text","size":"22"},
  {"cont":"","type":"text"},
  {"type":"div_line","cont":"含表头多列"},
  {"thead":{"名称":"50%","单价":"20%","数量":"15%","金额":"15%"},
   "tbody":[["番茄炒蛋",24,2,48],["西红柿炒番茄豆腐",24,7,168],["鸡蛋炒鸭蛋",24,1,24]]},
  {"type":"div_star","cont":"无表头多列"},
  {"thead":["60%","40%"],"tbody":[["序号1","内容AAA"],["序号2","内容BBBBBB"]]},
  {"type":"div_star","cont":"左右两列"},
  {"both_sides":["收款金额","258.00"]},
  {"both_sides":["支付方式","微信支付"]},
  {"type":"div_star","cont":"二维码"},
  {"cont":"http://www.sw-aiot.com","type":"qrcode","align":"center","size":"60"},
  {"type":"div_star","cont":"双二维码"},
  {"cont":["http://www.sw-aiot.com","1234567890123"],"type":"qrcode","align":"center","size":"06"},
  {"type":"div_star","cont":"条码"},
  {"cont":"123456789","type":"bc128","align":"center","size":"22"},
  {"cont":"123456789","type":"code39","align":"center","hri":2},
  {"type":"div_star","cont":"图片"},
  {"type":"png","cont":"http://linxiaoge.oss-cn-shanghai.aliyuncs.com/ic/add/test.png","align":"center"},
  {"cont":"","type":"text"}
]`)
