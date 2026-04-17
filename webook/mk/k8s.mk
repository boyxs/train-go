# K3s 部署全流程
# 用法：make -f mk/k8s.mk deploy
# 或加到根 Makefile：include mk/k8s.mk

IMAGE := train/webook:1.0.0

.PHONY: deploy clean help

# 完整部署流程（编译 → 打镜像 → 同步到 K3s → Apply → Rollout）
deploy:
	@echo "--- [1/5] Compiling Go binary (Linux/AMD64) ---"
	@rm -f webook
	@go mod tidy
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags=k8s -o webook .

	@echo "--- [2/5] Building Docker image ---"
	@docker rmi -f $(IMAGE) || true
	@docker build -t $(IMAGE) .

	@echo "--- [3/5] Sync image to K3s containerd ---"
	@docker save $(IMAGE) -o webook.tar
	@k3s ctr images import webook.tar
	@rm webook.tar

	@echo "--- [4/5] Apply K8s manifests + rollout restart ---"
	@kubectl apply -f k8s/k8s-webook-deployment.yaml
	@kubectl rollout restart deployment webook-record

	@echo "--- [5/5] Apply Service (NodePort) ---"
	@kubectl apply -f k8s/k8s-webook-service.yaml

	@echo "--- [OK] Deployment complete ---"
	@kubectl get pods -l app=webook-record
	@echo "--- Service Access INFO ---"
	@kubectl get svc webook-record

# 清理 K8s 资源
clean:
	@kubectl delete -f k8s/k8s-webook-deployment.yaml,k8s/k8s-webook-service.yaml --ignore-not-found

help:
	@echo "make -f mk/k8s.mk deploy  - 一键部署到 K3s"
	@echo "make -f mk/k8s.mk clean   - 清理 K8s 资源"

# 访问验证：
#   curl http://localhost:30908/hello
#
# Service 创建的另一种方式（如果不想用 YAML，用命令行创建）：
#   if ! kubectl get svc webook-record > /dev/null 2>&1; then
#     kubectl expose deployment webook-record \
#       --type=NodePort --port=8089 --target-port=8089 --name=webook-record
#   fi
