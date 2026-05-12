while read func; do
    # Search in all files except tile_view.go. Use -l to just get filenames.
    ext_usage=$(grep -r -l "$func" . --exclude=cmd/WiiUDownloader/tile_view.go --exclude=functions.txt --exclude=check_usage.sh --exclude=WiiUDownloader)
    
    # Search in tile_view.go but ignore the definition line.
    # We use a more robust regex for the definition.
    int_usage=$(grep "$func" cmd/WiiUDownloader/tile_view.go | grep -vE "func .*$func")

    if [ -z "$ext_usage" ] && [ -z "$int_usage" ]; then
        echo "$func"
    fi
done < functions.txt
