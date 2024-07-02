local micro = import("micro")

local open = {
    ['('] = ')',
    ['{'] = '}',
    ['['] = ']',
    ['"'] = '"',
    ['\''] = '\'',
    ['`'] = '`',
}

local close = {
    [')'] = true,
    ['}'] = true,
    [']'] = true,
    ['"'] = true,
    ['\''] = true,
    ['`'] = true,
}

local fmt = import("fmt")
function preInsert(bp, args)
    if #args[1] ~= 1 then
        return
    end
    if close[args[1]] and args[1] == bp:RuneAtCursor() then
        bp:MoveTo(bp:Cursor():Right(bp.Buffer).Pos)
        return false
    end
    return true
end

function onInsert(bp, args)
    if #args[1] ~= 1 then
        return
    end
    if open[args[1]] ~= nil then
        bp:InsertString(open[args[1]])
        bp:MoveTo(bp:Cursor():Left(bp.Buffer).Pos)
    end
end
micro.PostHook("insert", onInsert)
micro.PreHook("insert", preInsert)
