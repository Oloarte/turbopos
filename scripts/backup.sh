#!/bin/sh
echo "[Backup] Servicio iniciado"
while true; do
  sleep 86400
  FILENAME="/backups/backup_$(date +%Y%m%d_%H%M%S).sql"
  pg_dump -h postgres -U postgres turbopos > "$FILENAME"
  echo "[Backup] Completado: $FILENAME"
  # Mantener solo los 7 backups mas recientes
  ls -t /backups/backup_*.sql | tail -n +8 | xargs rm -f
  echo "[Backup] Limpieza completada. Archivos actuales:"
  ls -lh /backups/backup_*.sql 2>/dev/null
done
