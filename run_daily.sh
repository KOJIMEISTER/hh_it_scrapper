#!/bin/sh

# Path to the main executable
MAIN_EXECUTABLE="/app/main"

# Function to calculate yesterday's date
yesterday() {
    date -u -v-1d '+%Y-%m-%d' 2>/dev/null || \
    date -u --date="yesterday" '+%Y-%m-%d' || \
    echo "$(date -u '+%Y-%m-%d')"
}

# Function to calculate a date N days ago
get_date_days_ago() {
    local days=$1
    date -u -v-"${days}"d '+%Y-%m-%d' 2>/dev/null || \
    date -u --date="${days} days ago" '+%Y-%m-%d' || \
    echo "$(date -u '+%Y-%m-%d')"
}

# Download data for the past 30 days by specifying 30 separate 1-day ranges
echo "Starting data fetching for the past 30 days..."
for i in $(seq 1 30); do
    FROM_DATE=$(get_date_days_ago "$i")
    TO_DATE=$(get_date_days_ago "$((i - 1))")
    
    echo "Fetching data from $FROM_DATE to $TO_DATE..."
    $MAIN_EXECUTABLE --from "$FROM_DATE" --to "$TO_DATE"
    
    if [ $? -eq 0 ]; then
        echo "Successfully fetched data for $FROM_DATE."
    else
        echo "Error fetching data for $FROM_DATE. Check logs for details."
    fi
done
echo "Completed initial 30-day data fetching."

# Infinite loop to run the job daily
while true
do
    YESTERDAY=$(yesterday)
    TODAY=$(date '+%Y-%m-%d')
    
    echo "Starting daily vacancy fetching job from $YESTERDAY to $TODAY..."
    $MAIN_EXECUTABLE --from "$YESTERDAY" --to "$TODAY"
    
    if [ $? -eq 0 ]; then
        echo "Daily job completed successfully."
    else
        echo "Daily job encountered errors. Check logs for details."
    fi
    
    echo "Waiting for 24 hours before the next run..."
    sleep 86400  # Sleep for 24 hours
done
