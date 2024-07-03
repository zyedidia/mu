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
    if close[args[1]] and bp:RuneAt(bp:Cursor().Pos - 1):match("[%w_]") then
        return
    end
    if open[args[1]] ~= nil and not bp:RuneAtCursor():match("[%w_(]") then
        bp:InsertString(open[args[1]])
        bp:MoveTo(bp:Cursor():Left(bp.Buffer).Pos)
    end
end

function preRemove(bp, args)
    local amt = bp:Cursor().Pos - args[1]
    if amt == 1 and close[bp:RuneAtCursor()] and open[bp:RuneAt(bp:Cursor().Pos - 1)] then
        bp:Remove(bp:Cursor().Pos - 1, bp:Cursor().Pos + 1)
        return false
    end
    return true
end

micro.PostHook("insert", onInsert)
micro.PreHook("insert", preInsert)
micro.PreHook("remove-to", preRemove)
