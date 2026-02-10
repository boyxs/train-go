.PHONY: docker clean
docker:
	@echo "--- [1/5] Compiling Go binary ---"
	@rm webook || true
	@go mod tidy
	# Build for Linux/AMD64 to ensure compatibility inside the container
	@GOOS=linux GOARCH=amd64 go build -tags=k8s -o webook .

	@echo "--- [2/5] Building Docker image ---"
	@docker rmi -f train/webook:1.0.0 || true
	@docker build -t train/webook:1.0.0 .

	@echo "--- [3/5] Synchronizing image to K3s ---"
	# Exporting from Docker and importing to K3s containerd namespace
	@docker save train/webook:1.0.0 -o webook.tar
	@k3s ctr images import webook.tar
	@rm webook.tar

	@echo "--- [4/5] Deploying to Kubernetes ---"
	# Apply YAML configuration and force a rollout to pick up the new image
	@kubectl apply -f k8s-webook-deployment.yaml
	@kubectl rollout restart deployment webook-record

	@echo "--- [5/5] Managing Service (NodePort) ---"
	# Create service if it doesn't exist to expose the application
	@kubectl apply -f k8s-webook-service.yaml
	# 或者
	# @if ! kubectl get svc webook-record > /dev/null 2>&1; then \
	#	 kubectl expose deployment webook-record --type=NodePort --port=8089 --target-port=8089 --name=webook-record; \
	# fi


	@echo "--- [OK] Deployment complete! ---"
	@kubectl get pods -l app=webook-record
	@echo "--- Service Access INFO: ---"
	@kubectl get svc webook-record

clean:
	@kubectl delete -f k8s-webook-deployment.yaml,k8s-webook-service.yaml --ignore-not-found
# curl http://localhost:30908/hello

#.PHONY: docker
#docker:
#	@rm webook || true
#	@go mod tidy
#	@GOOS=linux GOARCH=amd64 go build -tags=k8s -o webook .
#	@docker rmi -f train/webook:1.0.0
#	@docker build -t train/webook:1.0.0 .
#
## make docker