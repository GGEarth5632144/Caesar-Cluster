#!/usr/bin/env bash
# สร้าง kubeconfig ที่ใช้สิทธิ์ของ ServiceAccount caesar-backend แล้ววางไว้ให้ container ใช้
#
# รันบนเครื่อง NUC หลัง apply deploy/k8s/caesar-backend-rbac.yaml แล้ว:
#   ./deploy/make-kubeconfig.sh
#
# ถ้า server ใน kubeconfig ของแอดมินเป็น 127.0.0.1 (container จะต่อไม่ได้) ให้ระบุเอง:
#   APISERVER=https://192.168.100.1:6443 ./deploy/make-kubeconfig.sh
set -euo pipefail

NS=caesar-system
SECRET=caesar-backend-token
OUT=${OUT:-./secrets/kubeconfig}
CONTAINER_UID=10001   # uid ของ user caesar ใน image ของ backend (ดู backend/Dockerfile)

if ! kubectl -n "$NS" get secret "$SECRET" >/dev/null 2>&1; then
  echo "ไม่พบ secret $SECRET ใน namespace $NS" >&2
  echo "ต้อง kubectl apply -f deploy/k8s/caesar-backend-rbac.yaml ก่อน" >&2
  exit 1
fi

APISERVER=${APISERVER:-$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')}

case "$APISERVER" in
  *127.0.0.1*|*localhost*)
    echo "!! API server เป็น $APISERVER ซึ่ง container จะต่อไม่ถึง" >&2
    echo "   รันใหม่โดยระบุ IP จริงของ NUC เช่น APISERVER=https://192.168.100.1:6443 $0" >&2
    exit 1
    ;;
esac

TOKEN=$(kubectl -n "$NS" get secret "$SECRET" -o jsonpath='{.data.token}' | base64 -d)
CA=$(kubectl -n "$NS" get secret "$SECRET" -o jsonpath='{.data.ca\.crt}')

if [ -z "$TOKEN" ] || [ -z "$CA" ]; then
  echo "secret $SECRET ยังไม่มี token/ca.crt — รอสักครู่แล้วรันใหม่" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUT")"
cat > "$OUT" <<EOF
apiVersion: v1
kind: Config
clusters:
  - name: caesar
    cluster:
      server: ${APISERVER}
      certificate-authority-data: ${CA}
contexts:
  - name: caesar
    context:
      cluster: caesar
      user: caesar-backend
current-context: caesar
users:
  - name: caesar-backend
    user:
      token: ${TOKEN}
EOF

chmod 600 "$OUT"

# container รันเป็น uid 10001 ไม่ใช่ root — ถ้าไม่เปลี่ยนเจ้าของ ไฟล์โหมด 600 ที่ root ถืออยู่
# จะถูกอ่านไม่ได้จากใน container แล้วขึ้น permission denied ตอน start
if command -v sudo >/dev/null 2>&1; then
  sudo chown "$CONTAINER_UID:$CONTAINER_UID" "$OUT"
else
  chown "$CONTAINER_UID:$CONTAINER_UID" "$OUT"
fi

echo "เขียน kubeconfig ที่ $OUT แล้ว (server: $APISERVER)"
echo "ทดสอบ: kubectl --kubeconfig $OUT auth can-i create namespaces"
