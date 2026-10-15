# Kubernetes manifests (optional)
#
# These are optional: `docker compose up -d --build` is the supported way to run
# the whole platform. The manifests exist so the same images can run on a cluster
# without rewriting configuration by hand.
#
# What you must fill in before applying:
#
#   1. `secret.example.yaml` → copy to `secret.yaml`, set JWT_SECRET (and the
#      credential fields for the datastores) and never commit it.
#   2. `configmap.yaml` → point MONGO_URI / REDIS_ADDR / RABBITMQ_URL at your
#      managed services. The defaults assume the in-cluster examples in
#      `infra.example.yaml`, which are single-replica and not production-grade.
#   3. Image names: replace `eproc/<service>:latest` with your registry paths, and
#      build with `docker build -f deploy/docker/Dockerfile.service
#      --build-arg SERVICE=<name> -t <registry>/<name>:<tag> .`
#
# Apply order:
#
#   kubectl apply -f deploy/k8s/namespace.yaml
#   kubectl apply -f deploy/k8s/configmap.yaml
#   kubectl apply -f deploy/k8s/secret.yaml
#   kubectl apply -f deploy/k8s/infra.example.yaml   # or your managed services
#   kubectl apply -f deploy/k8s/api.yaml
#   kubectl apply -f deploy/k8s/web.yaml
#   kubectl apply -f deploy/k8s/ingress.yaml
#
# The probes in api.yaml use the same /healthz and /readyz the services expose
# locally, so a rollout is gated on the same signal the compose stack uses.
