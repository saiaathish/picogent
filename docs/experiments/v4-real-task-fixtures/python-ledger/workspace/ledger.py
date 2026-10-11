def summarize(events):
    categories = list(dict.fromkeys(event["category"] for event in events))
    return [
        {
            "category": category,
            "total": sum(event["amount"] for event in events if event["category"] == category),
        }
        for category in categories
    ]
