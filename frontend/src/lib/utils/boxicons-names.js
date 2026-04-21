import css from 'boxicons/css/boxicons.min.css?raw'

const re = /\.(bx[sl]?-[\w-]+):before/g
const names = []
let m
while ((m = re.exec(css)) !== null) names.push(m[1])

export const BOXICON_NAMES = Object.freeze([...new Set(names)])
