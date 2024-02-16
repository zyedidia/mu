package buf

type jumpLoc struct {
	file string
	pos  int
}

type jumpStack struct {
	locations []jumpLoc
	cur       int
}

func (j *jumpStack) push(file string, pos int) {
	j.locations = j.locations[:j.cur]
	j.locations = append(j.locations, jumpLoc{
		file: file,
		pos:  pos,
	})
	j.cur++
}

func (j *jumpStack) prev() (jumpLoc, bool) {
	if j.cur <= 0 {
		return jumpLoc{}, false
	}
	j.cur--
	return j.locations[j.cur], true
}

func (j *jumpStack) next() (jumpLoc, bool) {
	if j.cur >= len(j.locations)-1 {
		return jumpLoc{}, false
	}
	j.cur++
	return j.locations[j.cur], true
}
