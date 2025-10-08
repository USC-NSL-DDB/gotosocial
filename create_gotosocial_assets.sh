#!/bin/bash
# create_gotosocial_assets_simple.sh

kubectl delete configmap gotosocial-assets 2>/dev/null || true

echo "Creating ConfigMap from web/assets directory..."

# Create a temporary script file
TEMP_SCRIPT=$(mktemp)
echo "kubectl create configmap gotosocial-assets \\" > "$TEMP_SCRIPT"

# Add each file with its path
find web/assets -type f | while read -r file; do
    rel_path=${file#web/assets/}
    echo "  --from-file='$rel_path=$file' \\" >> "$TEMP_SCRIPT"
done

# Remove the last backslash
sed -i '$ s/ \\$//' "$TEMP_SCRIPT"

echo "Generated command:"
cat "$TEMP_SCRIPT"
echo ""

# Execute the script
bash "$TEMP_SCRIPT"

# Clean up
rm "$TEMP_SCRIPT"

echo "ConfigMap created!"
kubectl describe configmap gotosocial-assets | head -20