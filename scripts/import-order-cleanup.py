import argparse

def cleanup_imports_and_return(imports):
    os_packages = []
    jaeger_packages = []
    thirdparty_packages = []
    for i in imports:
        if i.strip() == "":
            continue
        if i.find("github.com/jaegertracing/jaeger/") != -1:
            jaeger_packages.append(i)
        elif i.find(".com") != -1 or i.find(".net") != -1 or i.find(".org") != -1 or i.find(".in") != -1:
            thirdparty_packages.append(i)
        else:
            os_packages.append(i)

    l = []
    needs_new_line = False
    if os_packages:
        l.extend(os_packages)
        needs_new_line = True
    if thirdparty_packages:
        if needs_new_line:
            l.append("")
        l.extend(thirdparty_packages)
        needs_new_line = True
    if jaeger_packages:
        if needs_new_line:
            l.append("")
        l.extend(jaeger_packages)

    imports_reordered = imports != l
    l.insert(0, "import (")
    l.append(")")
    return l, imports_reordered

def parse_go_file(f):
    with open(f, 'r') as go_file:
        lines = [i.rstrip() for i in go_file.readlines()]
        in_import_block = False
        imports_reordered = False
        imports = []
        output_lines = []
        for line in lines:
            if in_import_block:
                endIdx = line.find(")")
                if endIdx != -1:
                    in_import_block = False
                    ordered_imports, imports_reordered = cleanup_imports_and_return(imports)
                    output_lines.extend(ordered_imports)
                    imports = []
                    continue
                imports.append(line)
            else:
                importIdx = line.find("import (")
                if importIdx != -1:
                    in_import_block = True
                    continue
                output_lines.append(line)
        output_lines.append("")
        return "\n".join(output_lines), imports_reordered


def main():
    parser = argparse.ArgumentParser(
        description='Tool to make cleaning up import orders easily')

    parser.add_argument('-o', '--output', default='stdout',
                        choices=['inplace', 'stdout'],
                        help='output target [default: stdout]')

    parser.add_argument('-t', '--target',
                        help='list of filenames to operate upon',
                        nargs='+',
                        required=True)

    args = parser.parse_args()
    output = args.output
    go_files = args.target

    for f in go_files:
        parsed, imports_reordered = parse_go_file(f)
        if output == "stdout" and imports_reordered:
            print f + " imports out of order"
        else:
            with open(f, 'w') as ofile:
                ofile.write(parsed)

if __name__ == '__main__':
    main()
