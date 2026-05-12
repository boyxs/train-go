# --- Configuration ---
KUBECTL  := kubectl
NAMESPACE := default

# --- Symbols ---
STEP := [>]
INFO := [-]

.PHONY: help all mysql redis clean status

help:
	@echo "Infrastructure Management Commands:"
	@echo "  make -f mk/infra.mk all         - Deploy all components"
	@echo "  make -f mk/infra.mk mysql       - Deploy/Restart MySQL"
	@echo "  make -f mk/infra.mk redis       - Deploy/Restart Redis"
	@echo "  make -f mk/infra.mk status      - Check pods status"

# --- One-click Deployment ---
all:
	@echo "$(STEP) Starting full infrastructure deployment..."
	@$(MAKE) -f mk/infra.mk mysql
	@$(MAKE) -f mk/infra.mk redis
	@echo "$(STEP) Done: All components deployed."

# --- MySQL Target ---
mysql:
	@echo "$(STEP) [1/5] Applying Storage (PV & PVC)..."
	@$(KUBECTL) apply -f ../kubernetes/mysql-pv.yaml -n $(NAMESPACE)
	@$(KUBECTL) apply -f ../kubernetes/mysql-pvc.yaml -n $(NAMESPACE)

	@echo "$(STEP) [2/5] Applying Manifests (Deployment & Service)..."
	@$(KUBECTL) apply -f ../kubernetes/mysql-deployment.yaml -n $(NAMESPACE)
	@$(KUBECTL) apply -f ../kubernetes/mysql-service.yaml -n $(NAMESPACE)

	@echo "$(STEP) [3/5] Triggering Restart to pick up changes..."
	@$(KUBECTL) rollout restart deployment webook-mysql -n $(NAMESPACE)

	@echo "$(STEP) [4/5] Waiting for MySQL to be Ready..."
	@$(KUBECTL) rollout status deployment webook-mysql -n $(NAMESPACE)

	@echo "$(STEP) [5/5] Final Verification..."
	@$(KUBECTL) get pods -l app=webook-mysql -n $(NAMESPACE)
	@$(KUBECTL) get pvc -n $(NAMESPACE) | grep webook-mysql-pvc
	@echo "$(INFO) MySQL is up and running!"

# --- Redis Target ---
redis:
	@echo "$(STEP) Step 1/3: Applying Redis manifests..."
	@$(KUBECTL) apply -f ../kubernetes/redis-deployment.yaml -n $(NAMESPACE)
	@$(KUBECTL) apply -f ../kubernetes/redis-service.yaml -n $(NAMESPACE)
	@echo "$(STEP) Step 2/3: Restarting webook-redis..."
	@$(KUBECTL) rollout restart deployment webook-redis -n $(NAMESPACE)
	@echo "$(STEP) Step 3/3: Waiting for stability..."
	@$(KUBECTL) rollout status deployment webook-redis -n $(NAMESPACE)
	@echo "$(INFO) Redis deployment finished."

# --- Etcd Target ---
etcd:
	@echo "$(STEP) Step 1/3: Applying Etcd manifests..."
	@$(KUBECTL) apply -f ../kubernetes/etcd-deployment.yaml -n $(NAMESPACE)
	@$(KUBECTL) apply -f ../kubernetes/etcd-service.yaml -n $(NAMESPACE)
	@echo "$(STEP) Step 2/3: Restarting webook-etcd..."
	@$(KUBECTL) rollout restart deployment webook-etcd -n $(NAMESPACE)
	@echo "$(STEP) Step 3/3: Waiting for stability..."
	@$(KUBECTL) rollout status deployment webook-etcd -n $(NAMESPACE)
	@echo "$(INFO) Etcd deployment finished."

# --- Observability ---
status:
	@echo "$(STEP) Fetching current status..."
	@$(KUBECTL) get pods,svc -l 'app in (mysql, redis, etcd)' -n $(NAMESPACE)

# --- Cleanup ---
clean:
	@echo "$(STEP) Cleaning up infrastructure..."
	@$(KUBECTL) delete -f ../kubernetes/mysql-deployment.yaml,../kubernetes/mysql-service.yaml,../kubernetes/redis-deployment.yaml,../kubernetes/redis-service.yaml,../kubernetes/etcd-deployment.yaml,../kubernetes/etcd-service.yaml -n $(NAMESPACE) --ignore-not-found
	@echo "$(INFO) Cleanup complete."

# make -f mk/infra.mk xxx