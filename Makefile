PLATFORM := "linux/amd64,linux/arm/v7,linux/arm64"

TAG?=v1
SERVER?=211.65.102.40:5001
OWNER?=jessestutler
NAME=faas-autoscaler

.PHONY: local-docker
build-local:
	@echo $(SERVER)/$(OWNER)/$(NAME):$(TAG) \
	&& docker buildx create --use --name=multiarch --node multiarch \
	&& docker buildx build \
		--progress=plain \
		--platform linux/amd64 \
		--output "type=docker,push=false" \
		--tag $(SERVER)/$(OWNER)/$(NAME):$(TAG) .

.PHONY: push-docker
push-docker:
	@echo $(SERVER)/$(OWNER)/$(NAME):$(TAG) \
	&& docker buildx create --use --name=multiarch --node multiarch \
	&& docker buildx build \
		--progress=plain \
		--platform $(PLATFORM) \
		--output "type=image,push=true" \
		--tag $(SERVER)/$(OWNER)/$(NAME):$(TAG) .