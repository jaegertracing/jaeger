import argparse
import fnmatch
import os
import sys
from os import path

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

    l = ["import ("]
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
    l.append(")")
    return l

def parse_go_file(f):
    with open(f, 'r') as go_file:
        lines = [i.rstrip() for i in go_file.readlines()]
        in_import_block = False
        imports = []
        output_lines = []
        for line in lines:
            if in_import_block:
                endIdx = line.find(")")
                if endIdx != -1:
                    in_import_block = False
                    output_lines.extend(cleanup_imports_and_return(imports))
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
        return "\n".join(output_lines)


def main():
    parser = argparse.ArgumentParser(
        description='Tool to make cleaning up import orders easily')

    parser.add_argument('-o', '--output', default='stdout',
                        choices=['inplace', 'stdout'],
                        help='output target [default: stdout]')

    parser.add_argument('-t', '--target',
                        help='comma seperated filenames to operate upon',
                        nargs='+')

    args = parser.parse_args()
    output = args.output

    go_files = []
    target = args.target
    if target:
        go_files = target
        print target
    else:
        print >>sys.stderr, "No input specified, operating upon all *.go files in local dir"
        dirs = get_local_packages()
        go_files = get_go_files(dirs)

    for f in go_files:
        print >>sys.stderr, "Parsing ", f
        parsed = parse_go_file(f)
        if output == "stdout":
            print parsed
        else:
            with open(f, 'w') as ofile:
                ofile.write(parsed)
            print >>sys.stderr, "updated in place"

if __name__ == '__main__':
    main()
