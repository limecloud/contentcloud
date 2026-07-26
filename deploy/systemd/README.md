# ContentCloud systemd 部署

该目录用于无 Docker 的 Linux 部署，目录和服务管理方式与 `sub2api-admin-plus` 保持一致：

```text
/opt/contentcloud/releases/<commit>/   版本化发布目录
/opt/contentcloud/current              当前版本软链接
/etc/contentcloud/contentcloud.env     root-only 环境配置
/var/lib/contentcloud                  本地对象数据
contentcloud-server.service            Web/API 服务
contentcloud-worker.service            确定性 Worker
```

Server 只监听 `127.0.0.1:18082`，生产流量由宝塔原生反向代理接入。PostgreSQL 不对公网开放，Server 与 Worker 使用同一个独立数据库账号。

Worker 当前仅需要以下 OCR 系统依赖：

```bash
apt-get install tesseract-ocr tesseract-ocr-chi-sim tesseract-ocr-eng
```

默认配置不安装 ClamAV，并设置 `CONTENTCLOUD_REQUIRE_MALWARE_SCAN=0`。开放不可信文件上传前，应另行评估并启用恶意文件扫描。

安装发布产物后：

```bash
install -d -o contentcloud -g contentcloud -m 0750 /var/lib/contentcloud
install -d -o root -g contentcloud -m 0750 /etc/contentcloud
install -m 0644 deploy/systemd/contentcloud-server.service /etc/systemd/system/
install -m 0644 deploy/systemd/contentcloud-worker.service /etc/systemd/system/
install -m 0640 deploy/systemd/contentcloud.env.example /etc/contentcloud/contentcloud.env
systemctl daemon-reload
systemctl enable --now contentcloud-server contentcloud-worker
```

上线前必须替换数据库密码，并确认 `/etc/contentcloud/contentcloud.env` 不对其他用户可读。
