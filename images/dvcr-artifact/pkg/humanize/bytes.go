package humanize

func HumanizeBytes(num int64) string {
	return humanizeQuantity4(num, BIBase) + "B"
}
